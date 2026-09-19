// Package direct provides the local tool loop for API-backed coding tasks.
package direct

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/pausan/agenttik/app/internal/agent"
)

// Client is immutable during a turn. Credentials never enter tool environments.
type Client struct {
	HTTP          *http.Client
	Endpoint      string
	Protocol      string
	Key           string
	HistoryDir    string
	Headers       map[string]string
	SessionHeader string
}

type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Extra    json.RawMessage `json:"extra_content,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type Message struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	Reasoning        string           `json:"reasoning_content,omitempty"`
	ReasoningField   string           `json:"reasoning_field,omitempty"`
	ReasoningDetails []map[string]any `json:"reasoning_details,omitempty"`
	Calls            []ToolCall       `json:"tool_calls,omitempty"`
	CallID           string           `json:"tool_call_id,omitempty"`
	ToolError        bool             `json:"tool_error,omitempty"`
	Blocks           []contentBlock   `json:"native_blocks,omitempty"`
}
type conversation struct {
	WorkDir  string    `json:"work_dir"`
	Messages []Message `json:"messages"`
}

func (p *Client) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if err := req.CheckWorkDir(); err != nil {
		return nil, err
	}
	if p.HTTP == nil {
		return nil, fmt.Errorf("API HTTP client is required")
	}
	id := req.ProviderSessionID
	if req.Isolated || !strings.HasPrefix(id, "direct-") {
		id = "direct-" + uuid.NewString()
	}
	if _, err := uuid.Parse(strings.TrimPrefix(id, "direct-")); err != nil {
		return nil, fmt.Errorf("invalid API conversation id")
	}
	path := filepath.Join(p.HistoryDir, id+".json")
	history := conversation{WorkDir: req.WorkDir}
	if !req.Isolated && req.ProviderSessionID == id {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("cannot resume API conversation: %w", err)
		}
		err = json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&history)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot read API conversation")
		}
		if history.WorkDir != req.WorkDir {
			return nil, fmt.Errorf("API conversation belongs to a different work folder; start a new task")
		}
	}
	history.Messages = append(history.Messages, Message{Role: "user", Content: req.Prompt})
	key := p.Key
	out := make(chan agent.Event, 64)
	go func() {
		defer close(out)
		emit := func(e agent.Event) {
			select {
			case out <- e:
			case <-ctx.Done():
			}
		}
		emit(agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: id})
		usage := agent.Usage{}
		err := p.loop(ctx, req, id, key, &history, &usage, emit)
		if !req.Isolated {
			b, marshalErr := json.Marshal(history)
			if marshalErr != nil {
				err = marshalErr
			} else if len(b) > 16<<20 {
				err = fmt.Errorf("API conversation is too large; start a new task")
			} else {
				if saveErr := PrivateWrite(path, b); saveErr != nil {
					err = fmt.Errorf("save API conversation: %w", saveErr)
				}
			}
		}
		if err != nil {
			emit(agent.Event{Type: agent.EventError, Text: err.Error()})
			return
		}
		emit(agent.Event{Type: agent.EventDone, Usage: &usage})
	}()
	return out, nil
}
func (p *Client) loop(ctx context.Context, req agent.TurnRequest, id, key string, history *conversation, total *agent.Usage, emit func(agent.Event)) error {
	metadata := req.Isolated && req.Permission != agent.PermissionWorkspace && req.Permission != agent.PermissionFull
	env := environment{}
	if !metadata {
		env = detectEnvironment()
	}
	system := systemPrompt(req, env)
	for step := 0; step < 64; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		msgs := append([]Message{{Role: "system", Content: system}}, history.Messages...)
		body := map[string]any{"model": req.Model, "messages": chatMessages(msgs), "stream": true, "stream_options": map[string]bool{"include_usage": true}}
		if !metadata {
			body["tools"] = codingTools(req.Permission, env)
		}
		if p.Protocol == "anthropic" {
			body = anthropicBody(req, system, history.Messages, body["tools"])
		}
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if len(b) > 16<<20 {
			return fmt.Errorf("API conversation is too large; start a new task")
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.Endpoint, bytes.NewReader(b))
		if err != nil {
			return err
		}
		httpReq.Header.Set("Authorization", "Bearer "+key)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", "agenttik/1.0")
		for name, value := range p.Headers {
			httpReq.Header.Set(name, value)
		}
		if p.SessionHeader != "" {
			httpReq.Header.Set(p.SessionHeader, id)
		}
		if p.Protocol == "anthropic" {
			httpReq.Header.Del("Authorization")
			httpReq.Header.Set("x-api-key", key)
			httpReq.Header.Set("anthropic-version", "2023-06-01")
		}
		response, err := p.HTTP.Do(httpReq)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("API returned HTTP %d; check your key, model access and billing in the provider console", response.StatusCode)
		}
		var reply Message
		var usage agent.Usage
		if p.Protocol == "anthropic" {
			reply, usage, err = readAnthropic(response.Body, emit)
		} else {
			reply, usage, err = ReadCompletion(response.Body, emit)
		}
		response.Body.Close()
		if err != nil {
			return err
		}
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.CacheReadTokens += usage.CacheReadTokens
		total.CacheWriteTokens += usage.CacheWriteTokens
		total.CostUSD += usage.CostUSD
		total.ContextTokens = usage.ContextTokens
		emit(agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{ContextTokens: usage.ContextTokens}})
		for _, call := range reply.Calls {
			if call.ID == "" || call.Function.Name == "" || !json.Valid([]byte(call.Function.Arguments)) {
				return fmt.Errorf("API returned an incomplete tool call")
			}
		}
		if metadata && len(reply.Calls) > 0 {
			return fmt.Errorf("unexpected tool request in an isolated turn")
		}
		history.Messages = append(history.Messages, reply)
		if len(reply.Calls) == 0 {
			return nil
		}
		for _, call := range reply.Calls {
			emit(agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{ID: call.ID, Name: call.Function.Name, Input: call.Function.Arguments}})
			result, toolErr := runTool(ctx, req, call, env)
			if toolErr != nil {
				result = toolErr.Error()
			}
			emit(agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{ID: call.ID, Name: call.Function.Name, Output: result, IsError: toolErr != nil}})
			history.Messages = append(history.Messages, Message{Role: "tool", Content: result, CallID: call.ID, ToolError: toolErr != nil})
		}
	}
	return fmt.Errorf("API reached the 64-step turn limit; continue with another prompt")
}

func ReadCompletion(r io.Reader, emit func(agent.Event)) (reply Message, usage agent.Usage, err error) {
	reply.Role = "assistant"
	var content, reasoning strings.Builder
	defer func() { reply.Content, reply.Reasoning = content.String(), reasoning.String() }()
	scanner := bufio.NewScanner(io.LimitReader(r, 16<<20))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	finished := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if !finished {
				return reply, usage, fmt.Errorf("API stream ended without a completion")
			}
			return reply, usage, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string           `json:"content"`
					Reasoning        string           `json:"reasoning_content"`
					ReasoningAlt     string           `json:"reasoning"`
					ReasoningDetails []map[string]any `json:"reasoning_details"`
					Calls            []struct {
						Index    int             `json:"index"`
						ID       string          `json:"id"`
						Extra    json.RawMessage `json:"extra_content"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				Input   int64 `json:"prompt_tokens"`
				Output  int64 `json:"completion_tokens"`
				Details struct {
					Cached int64 `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return reply, usage, fmt.Errorf("invalid API stream")
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return reply, usage, fmt.Errorf("API reported a stream error")
		}
		if chunk.Usage.Input > 0 || chunk.Usage.Output > 0 {
			usage = agent.Usage{InputTokens: chunk.Usage.Input - chunk.Usage.Details.Cached, OutputTokens: chunk.Usage.Output, CacheReadTokens: chunk.Usage.Details.Cached, ContextTokens: chunk.Usage.Input}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Finish != "" {
			if choice.Finish != "stop" && choice.Finish != "tool_calls" {
				return reply, usage, fmt.Errorf("API completion was truncated or refused (%s)", safeFinish(choice.Finish))
			}
			finished = true
		}
		if text := choice.Delta.Content; text != "" {
			content.WriteString(text)
			emit(agent.Event{Type: agent.EventText, Text: text})
		}
		if choice.Delta.ReasoningAlt != "" {
			reply.ReasoningField = "reasoning"
		}
		for _, detail := range choice.Delta.ReasoningDetails {
			if err := mergeReasoning(&reply, detail); err != nil {
				return reply, usage, err
			}
		}
		if choice.Delta.Reasoning == "" {
			choice.Delta.Reasoning = choice.Delta.ReasoningAlt
		}
		if text := choice.Delta.Reasoning; text != "" {
			reasoning.WriteString(text)
			emit(agent.Event{Type: agent.EventThinking, Text: text})
		}
		for _, delta := range choice.Delta.Calls {
			if delta.Index < 0 || delta.Index > 63 {
				return reply, usage, fmt.Errorf("invalid API tool index")
			}
			for len(reply.Calls) <= delta.Index {
				reply.Calls = append(reply.Calls, ToolCall{Type: "function"})
			}
			call := &reply.Calls[delta.Index]
			call.ID += delta.ID
			if len(delta.Extra) > 0 {
				call.Extra = delta.Extra
			}
			call.Function.Name += delta.Function.Name
			call.Function.Arguments += delta.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return reply, usage, err
	}
	return reply, usage, fmt.Errorf("API stream was interrupted")
}

// Local history metadata is never sent as unsupported API fields.
func chatMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		wire := map[string]any{"role": msg.Role, "content": msg.Content}
		if msg.Reasoning != "" {
			field := "reasoning_content"
			if msg.ReasoningField == "reasoning" {
				field = "reasoning"
			}
			wire[field] = msg.Reasoning
		}
		if len(msg.ReasoningDetails) > 0 {
			wire["reasoning_details"] = msg.ReasoningDetails
		}
		if len(msg.Calls) > 0 {
			wire["tool_calls"] = msg.Calls
		}
		if msg.CallID != "" {
			wire["tool_call_id"] = msg.CallID
		}
		out = append(out, wire)
	}
	return out
}

func safeFinish(reason string) string {
	switch reason {
	case "length", "content_filter":
		return reason
	}
	return "unknown stop reason"
}

func mergeReasoning(reply *Message, delta map[string]any) error {
	for _, detail := range reply.ReasoningDetails {
		index, hasIndex := delta["index"].(float64)
		id, hasID := delta["id"].(string)
		if stringField(detail, "type") != stringField(delta, "type") || !((hasIndex && detail["index"] == index) || (hasID && id != "" && detail["id"] == id)) {
			continue
		}
		for key, value := range delta {
			if value == nil {
				continue
			}
			switch key {
			case "text", "summary", "data", "signature":
				before, _ := detail[key].(string)
				next, _ := value.(string)
				detail[key] = before + next
			default:
				detail[key] = value
			}
		}
		return nil
	}
	if len(reply.ReasoningDetails) >= 256 {
		return fmt.Errorf("too many reasoning blocks")
	}
	reply.ReasoningDetails = append(reply.ReasoningDetails, delta)
	return nil
}

func stringField(m map[string]any, key string) string { value, _ := m[key].(string); return value }
