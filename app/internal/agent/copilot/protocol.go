package copilot

import "encoding/json"

// envelope is one event from copilot --output-format json. The CLI's event
// schema is additive, so only the event type and the data for events we use
// are decoded here.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type sessionStart struct {
	SessionID string `json:"sessionId"`
}

type sessionError struct {
	Message   string `json:"message"`
	ErrorType string `json:"errorType"`
}

type sessionUsageInfo struct {
	TokenLimit    int64 `json:"tokenLimit"`
	CurrentTokens int64 `json:"currentTokens"`
}

type assistantMessageDelta struct {
	MessageID    string `json:"messageId"`
	DeltaContent string `json:"deltaContent"`
}

type assistantMessage struct {
	MessageID     string `json:"messageId"`
	Content       string `json:"content"`
	ReasoningText string `json:"reasoningText"`
}

type assistantReasoningDelta struct {
	ReasoningID  string `json:"reasoningId"`
	DeltaContent string `json:"deltaContent"`
}

type assistantReasoning struct {
	ReasoningID string `json:"reasoningId"`
	Content     string `json:"content"`
}

type assistantUsage struct {
	Model            string                   `json:"model"`
	InputTokens      int64                    `json:"inputTokens"`
	OutputTokens     int64                    `json:"outputTokens"`
	CacheReadTokens  int64                    `json:"cacheReadTokens"`
	CacheWriteTokens int64                    `json:"cacheWriteTokens"`
	CopilotUsage     copilotUsage             `json:"copilotUsage"`
	QuotaSnapshots   map[string]quotaSnapshot `json:"quotaSnapshots"`
}

type copilotUsage struct {
	// Copilot reports AI credits in nano-AIU. One AI credit is one cent.
	TotalNanoAIU float64 `json:"totalNanoAiu"`
}

type toolExecutionStart struct {
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	MCPServerName string          `json:"mcpServerName"`
	Arguments     json.RawMessage `json:"arguments"`
}

type toolExecutionComplete struct {
	ToolCallID string          `json:"toolCallId"`
	Result     json.RawMessage `json:"result"`
	Error      json.RawMessage `json:"error"`
	Success    bool            `json:"success"`
}

func unmarshalData(data json.RawMessage, target any) bool {
	return len(data) > 0 && json.Unmarshal(data, target) == nil
}

func rawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
}
