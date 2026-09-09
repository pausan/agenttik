package claudecode

import (
	"encoding/json"
	"strings"

	"github.com/pausan/agenttik/app/internal/agent"
)

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

	// type=rate_limit_event
	RateLimitInfo *rateLimitInfo `json:"rate_limit_info"`

	// type=result
	IsError      bool                  `json:"is_error"`
	Result       string                `json:"result"`
	TotalCostUSD float64               `json:"total_cost_usd"`
	Usage        usage                 `json:"usage"`
	ModelUsage   map[string]modelUsage `json:"modelUsage"`
}

// modelUsage is the per-model breakdown on a result line. Only the window and
// the model's own name are read: the window is the CLI's own figure for the
// model it just ran, so it follows a 1M-context variant or an --autocompact
// ceiling without a code change here.
type modelUsage struct {
	ContextWindow  int64  `json:"contextWindow"`
	CanonicalModel string `json:"canonicalModel"`
}

// contextWindow is the window the CLI reported for the session's own model.
//
// A turn routinely runs more than one: auto mode classifies with a small
// model, and a subagent can use another again. Only the main conversation's
// window describes what the gauge measures, so the alias we asked for is
// matched against the canonical ids the CLI reports — a haiku session read
// against a 1M window looks far emptier than it is. The largest window is the
// fallback when nothing matches, so an unrecognised alias never shrinks the
// gauge below the conversation's real ceiling.
func (e envelope) contextWindow(model string) int64 {
	alias := strings.ToLower(model)
	// "opus[1m]" and friends name a variant of the same alias.
	if i := strings.IndexByte(alias, '['); i > 0 {
		alias = alias[:i]
	}
	var mine, largest int64
	for id, m := range e.ModelUsage {
		if m.ContextWindow > largest {
			largest = m.ContextWindow
		}
		if alias == "" || m.ContextWindow <= mine {
			continue
		}
		if strings.Contains(strings.ToLower(m.CanonicalModel), alias) ||
			strings.Contains(strings.ToLower(id), alias) {
			mine = m.ContextWindow
		}
	}
	if mine > 0 {
		return mine
	}
	return largest
}

// rateLimitInfo is the subscription allowance the CLI volunteers on a
// rate_limit_event line. It describes one bucket — whichever is nearest its
// ceiling — not the whole allowance, and only arrives once a turn has crossed
// a warning threshold.
type rateLimitInfo struct {
	Status         string  `json:"status"`
	RateLimitType  string  `json:"rateLimitType"`
	Utilization    float64 `json:"utilization"`
	ResetsAt       int64   `json:"resetsAt"`
	IsUsingOverage bool    `json:"isUsingOverage"`
}

// windowLabels names the buckets the CLI reports. The event carries no window
// duration, so the type is the only thing to label it with; an unseen type is
// shown as it arrived rather than guessed at.
var windowLabels = map[string]string{
	"five_hour": "5-hour",
	"seven_day": "Weekly",
	"session":   "Session",
	"weekly":    "Weekly",
	"overage":   "Overage",
}

// limits converts one rate_limit_event into the shape every provider reports
// its allowance in. Utilization is a fraction; the panel reads percent.
func (r rateLimitInfo) limits() []agent.RateLimit {
	label := windowLabels[r.RateLimitType]
	if label == "" && r.RateLimitType != "" {
		label = strings.ToUpper(r.RateLimitType[:1]) + strings.ReplaceAll(r.RateLimitType[1:], "_", " ")
	}
	if label == "" {
		label = "Allowance"
	}
	return []agent.RateLimit{{
		LimitID:     r.RateLimitType,
		LimitName:   label,
		ReachedType: reachedType(r),
		Primary: &agent.RateLimitWindow{
			Label:       label,
			UsedPercent: r.Utilization * 100,
			ResetsAt:    r.ResetsAt,
		},
	}}
}

// reachedType turns the CLI's own status into the note the panel shows. Only a
// state worth reading about is returned; the ordinary case says nothing.
func reachedType(r rateLimitInfo) string {
	switch {
	case r.IsUsingOverage:
		return "using overage"
	case r.Status == "allowed":
		return ""
	default:
		return r.Status
	}
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
