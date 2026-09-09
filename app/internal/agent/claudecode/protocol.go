package claudecode

import "encoding/json"

// envelope covers every line shape the CLI emits on stdout with
// --output-format stream-json. Unknown fields and unknown line types are
// ignored, so a CLI update adding events does not break parsing.
type envelope struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	SessionID string `json:"session_id"`

	// type=stream_event
	Event *streamEvent `json:"event"`

	// type=assistant | user
	Message message `json:"message"`

	// type=result
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        usage   `json:"usage"`
}

type message struct {
	Content []contentBlock `json:"content"`
	Usage   usage          `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`

	// tool_use
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// tool_result
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type streamEvent struct {
	Type  string `json:"type"`
	Delta *delta `json:"delta"`
}

type delta struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// contextTokens is how much of the model's window one request filled: the
// prompt, whether it was cached or sent again.
func (u usage) contextTokens() int64 {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}
