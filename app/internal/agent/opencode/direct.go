package opencode

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

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Reasoning string     `json:"reasoning_content,omitempty"`
	Calls     []toolCall `json:"tool_calls,omitempty"`
	CallID    string     `json:"tool_call_id,omitempty"`
}
type conversation struct {
	WorkDir  string    `json:"work_dir"`
	Messages []message `json:"messages"`
}

func (p *Provider) runDirect(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	id := req.ProviderSessionID
	if req.Isolated || !strings.HasPrefix(id, "direct-") {
		id = "direct-" + uuid.NewString()
	}
	if _, err := uuid.Parse(strings.TrimPrefix(id, "direct-")); err != nil {
		return nil, fmt.Errorf("invalid OpenCode conversation id")
	}
	path := filepath.Join(p.home(req.AccountHome), "opencode", "agenttik-sessions", id+".json")
	history := conversation{WorkDir: req.WorkDir}
	if !req.Isolated && req.ProviderSessionID == id {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("cannot resume OpenCode conversation: %w", err)
		}
		err = json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&history)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot read OpenCode conversation")
		}
		if history.WorkDir != req.WorkDir {
			return nil, fmt.Errorf("OpenCode conversation belongs to a different work folder; start a new task")
		}
	}
	history.Messages = append(history.Messages, message{Role: "user", Content: req.Prompt})
	key := p.key(req.AccountHome)
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
		err := p.directLoop(ctx, req, id, key, &history, &usage, emit)
		if !req.Isolated && err == nil {
			b, marshalErr := json.Marshal(history)
			if marshalErr != nil {
				err = marshalErr
			} else if len(b) > 16<<20 {
				err = fmt.Errorf("OpenCode conversation is too large; start a new task")
			} else {
				err = privateWrite(path, b)
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
func (p *Provider) directLoop(ctx context.Context, req agent.TurnRequest, id, key string, history *conversation, total *agent.Usage, emit func(agent.Event)) error {
	for step := 0; step < 64; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		system := "You are agenttik, a coding agent working in the user's project. Use the provided tools to inspect and change code. Follow AGENTS.md instructions found in the project. Paths are relative to the project. Do not claim actions you have not performed."
		if req.Isolated {
			system = "Answer the coding task metadata request concisely. Do not use tools."
		}
		msgs := append([]message{{Role: "system", Content: system}}, history.Messages...)
		body := map[string]any{"model": req.Model, "messages": msgs, "stream": true, "stream_options": map[string]bool{"include_usage": true}}
		if !req.Isolated {
			body["tools"] = codingTools(req.Permission)
		}
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if len(b) > 16<<20 {
			return fmt.Errorf("OpenCode conversation is too large; start a new task")
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(b))
		if err != nil {
			return err
		}
		httpReq.Header.Set("Authorization", "Bearer "+key)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", "agenttik/1.0")
		httpReq.Header.Set("x-opencode-session", id)
		response, err := p.client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("OpenCode request failed: %w", err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("OpenCode Go returned HTTP %d; check your key, subscription and usage in the OpenCode console", response.StatusCode)
		}
		reply, usage, err := readCompletion(response.Body, emit)
		response.Body.Close()
		if err != nil {
			return err
		}
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.CacheReadTokens += usage.CacheReadTokens
		total.ContextTokens = usage.ContextTokens
		emit(agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{ContextTokens: usage.ContextTokens}})
		history.Messages = append(history.Messages, reply)
		if len(reply.Calls) == 0 {
			return nil
		}
		if req.Isolated {
			return fmt.Errorf("unexpected tool request in an isolated turn")
		}
		for _, call := range reply.Calls {
			if err := ctx.Err(); err != nil {
				return err
			}
			emit(agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{ID: call.ID, Name: call.Function.Name, Input: call.Function.Arguments}})
			result, toolErr := runTool(ctx, req, call)
			if toolErr != nil {
				result = toolErr.Error()
			}
			emit(agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{ID: call.ID, Name: call.Function.Name, Output: result, IsError: toolErr != nil}})
			history.Messages = append(history.Messages, message{Role: "tool", Content: result, CallID: call.ID})
		}
	}
	return fmt.Errorf("OpenCode reached the 64-step turn limit; continue with another prompt")
}

func readCompletion(r io.Reader, emit func(agent.Event)) (reply message, usage agent.Usage, err error) {
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
				return reply, usage, fmt.Errorf("OpenCode stream ended without a completion")
			}
			return reply, usage, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					Calls     []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
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
			return reply, usage, fmt.Errorf("invalid OpenCode stream")
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return reply, usage, fmt.Errorf("OpenCode reported a stream error")
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
				return reply, usage, fmt.Errorf("OpenCode completion stopped: %s", choice.Finish)
			}
			finished = true
		}
		if text := choice.Delta.Content; text != "" {
			content.WriteString(text)
			emit(agent.Event{Type: agent.EventText, Text: text})
		}
		if text := choice.Delta.Reasoning; text != "" {
			reasoning.WriteString(text)
			emit(agent.Event{Type: agent.EventThinking, Text: text})
		}
		for _, delta := range choice.Delta.Calls {
			if delta.Index < 0 || delta.Index > 63 {
				return reply, usage, fmt.Errorf("invalid OpenCode tool index")
			}
			for len(reply.Calls) <= delta.Index {
				reply.Calls = append(reply.Calls, toolCall{Type: "function"})
			}
			call := &reply.Calls[delta.Index]
			call.ID += delta.ID
			call.Function.Name += delta.Function.Name
			call.Function.Arguments += delta.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return reply, usage, err
	}
	return reply, usage, fmt.Errorf("OpenCode stream was interrupted")
}
