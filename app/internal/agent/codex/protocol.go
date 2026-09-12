package codex

import (
	"encoding/json"
	"strings"

	"github.com/pausan/agenttik/app/internal/agent"
)

// envelope covers the JSONL event shapes documented for codex exec --json.
// Unknown fields and event types are ignored so additive CLI changes remain
// compatible.
type envelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     *item           `json:"item"`
	Usage    *usage          `json:"usage"`
	Error    json.RawMessage `json:"error"`
	Message  string          `json:"message"`
}

type item struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Summary          json.RawMessage `json:"summary"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	ExitCode         *int            `json:"exit_code"`
	Status           string          `json:"status"`
	Name             string          `json:"name"`
	Arguments        json.RawMessage `json:"arguments"`
	Input            json.RawMessage `json:"input"`
	Output           json.RawMessage `json:"output"`
	Query            string          `json:"query"`
}

type usage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

func (u usage) agentUsage() agent.Usage {
	return agent.Usage{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		CacheReadTokens: u.CachedInputTokens,
	}
}

func (e envelope) errorText() string {
	if e.Message != "" {
		return e.Message
	}
	if len(e.Error) == 0 || string(e.Error) == "null" {
		return ""
	}
	var message string
	if json.Unmarshal(e.Error, &message) == nil {
		return message
	}
	var detail struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(e.Error, &detail) == nil && detail.Message != "" {
		return detail.Message
	}
	return string(e.Error)
}

func (i item) failed() bool {
	return i.Status == "failed" || i.ExitCode != nil && *i.ExitCode != 0
}

func (i item) toolName() string {
	if i.Name != "" {
		return i.Name
	}
	return "MCP tool"
}

func (i item) toolInput() string {
	if len(i.Arguments) > 0 {
		return string(i.Arguments)
	}
	return string(i.Input)
}

func (i item) toolOutput() string {
	return string(i.Output)
}

func (i item) reasoningText() string {
	if i.Text != "" {
		return i.Text
	}
	if len(i.Summary) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(i.Summary, &text) == nil {
		return text
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(i.Summary, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return string(i.Summary)
}
