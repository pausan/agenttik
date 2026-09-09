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
	IsError      bool                  `json:"is_error"`
	Result       string                `json:"result"`
	TotalCostUSD float64               `json:"total_cost_usd"`
	Usage        usage                 `json:"usage"`
	ModelUsage   map[string]modelUsage `json:"modelUsage"`
}

// modelUsage is the per-model breakdown on a result line. Only the window is
// read: it is the CLI's own figure for the model it just ran, so it follows a
// 1M-context variant or an --autocompact ceiling without a code change here.
type modelUsage struct {
	ContextWindow int64 `json:"contextWindow"`
}

// contextWindow is the largest window any model reported for the turn. A turn
// normally runs one model; a subagent on a smaller one must not shrink the
// gauge the main conversation is measured against.
func (e envelope) contextWindow() int64 {
	var window int64
	for _, m := range e.ModelUsage {
		if m.ContextWindow > window {
			window = m.ContextWindow
		}
	}
	return window
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
