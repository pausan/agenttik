package direct

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pausan/agenttik/app/internal/agent"
)

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Data      string          `json:"data,omitempty"`
}

func anthropicBody(req agent.TurnRequest, system string, history []Message, tools any) map[string]any {
	messages := []map[string]any{}
	for _, msg := range history {
		if msg.Role == "tool" {
			result := map[string]any{"type": "tool_result", "tool_use_id": msg.CallID, "content": msg.Content, "is_error": msg.ToolError}
			if len(messages) > 0 && messages[len(messages)-1]["role"] == "user" {
				last := messages[len(messages)-1]
				last["content"] = append(last["content"].([]any), result)
			} else {
				messages = append(messages, map[string]any{"role": "user", "content": []any{result}})
			}
			continue
		}
		blocks := []any{}
		if len(msg.Blocks) > 0 {
			for _, block := range msg.Blocks {
				blocks = append(blocks, block)
			}
		} else {
			if msg.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, call := range msg.Calls {
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": json.RawMessage(call.Function.Arguments)})
			}
		}
		messages = append(messages, map[string]any{"role": msg.Role, "content": blocks})
	}
	body := map[string]any{"model": req.Model, "system": system, "messages": messages, "max_tokens": 8192, "stream": true}
	if tools != nil {
		native := []any{}
		for _, tool := range tools.([]any) {
			fn := tool.(map[string]any)["function"].(map[string]any)
			native = append(native, map[string]any{"name": fn["name"], "description": fn["description"], "input_schema": fn["parameters"]})
		}
		body["tools"] = native
	}
	return body
}

func readAnthropic(r io.Reader, emit func(agent.Event)) (Message, agent.Usage, error) {
	reply := Message{Role: "assistant"}
	usage := agent.Usage{}
	blocks := []contentBlock{}
	arguments := []string{}
	finish := ""
	scanner := bufio.NewScanner(io.LimitReader(r, 16<<20))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Type    string       `json:"type"`
			Index   int          `json:"index"`
			Block   contentBlock `json:"content_block"`
			Message struct {
				Usage struct {
					Input      int64 `json:"input_tokens"`
					Output     int64 `json:"output_tokens"`
					CacheRead  int64 `json:"cache_read_input_tokens"`
					CacheWrite int64 `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				Output int64 `json:"output_tokens"`
			} `json:"usage"`
			Delta struct {
				Type      string `json:"type"`
				Text      string `json:"text"`
				Partial   string `json:"partial_json"`
				Thinking  string `json:"thinking"`
				Signature string `json:"signature"`
				Stop      string `json:"stop_reason"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			return reply, usage, fmt.Errorf("invalid Anthropic stream")
		}
		switch event.Type {
		case "message_start":
			u := event.Message.Usage
			usage = agent.Usage{InputTokens: u.Input, OutputTokens: u.Output, CacheReadTokens: u.CacheRead, CacheWriteTokens: u.CacheWrite, ContextTokens: u.Input + u.CacheRead + u.CacheWrite}
		case "content_block_start":
			if event.Index != len(blocks) || event.Index >= 64 {
				return reply, usage, fmt.Errorf("invalid Anthropic content index")
			}
			blocks = append(blocks, event.Block)
			arguments = append(arguments, "")
			if event.Block.Text != "" {
				emit(agent.Event{Type: agent.EventText, Text: event.Block.Text})
			}
		case "content_block_delta":
			if event.Index < 0 || event.Index >= len(blocks) {
				return reply, usage, fmt.Errorf("invalid Anthropic delta index")
			}
			block := &blocks[event.Index]
			switch event.Delta.Type {
			case "text_delta":
				block.Text += event.Delta.Text
				emit(agent.Event{Type: agent.EventText, Text: event.Delta.Text})
			case "input_json_delta":
				arguments[event.Index] += event.Delta.Partial
			case "thinking_delta":
				block.Thinking += event.Delta.Thinking
				emit(agent.Event{Type: agent.EventThinking, Text: event.Delta.Thinking})
			case "signature_delta":
				block.Signature += event.Delta.Signature
			}
		case "message_delta":
			finish = event.Delta.Stop
			usage.OutputTokens = event.Usage.Output
		case "message_stop":
			if finish != "end_turn" && finish != "tool_use" && finish != "stop_sequence" {
				return reply, usage, fmt.Errorf("Anthropic completion was truncated or refused")
			}
			for i, block := range blocks {
				reply.Content += block.Text
				if block.Type == "tool_use" {
					if arguments[i] != "" {
						block.Input = json.RawMessage(arguments[i])
					}
					if !json.Valid(block.Input) {
						return reply, usage, fmt.Errorf("invalid Anthropic tool arguments")
					}
					call := ToolCall{ID: block.ID, Type: "function"}
					call.Function.Name, call.Function.Arguments = block.Name, string(block.Input)
					reply.Calls = append(reply.Calls, call)
				}
				reply.Blocks = append(reply.Blocks, block)
			}
			return reply, usage, nil
		case "error":
			return reply, usage, fmt.Errorf("Anthropic reported a stream error; check provider status and usage")
		}
	}
	if err := scanner.Err(); err != nil {
		return reply, usage, err
	}
	return reply, usage, fmt.Errorf("Anthropic stream was interrupted")
}
