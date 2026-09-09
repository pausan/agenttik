// Package agent defines the provider-neutral contract for running one agent
// turn. Implementations drive locally installed CLIs (Claude Code, Codex) or,
// later, key-based HTTP APIs. Nothing here knows about HTTP or SQL.
package agent

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("provider not implemented")

// Permission is how much the agent may do without being asked. A web UI cannot
// answer an interactive approval, so this is decided before the turn starts.
type Permission string

const (
	PermissionPlan      Permission = "plan"      // read and propose only
	PermissionWorkspace Permission = "workspace" // edit inside the project folder
	PermissionFull      Permission = "full"      // no guardrails
)

// Valid reports whether p is a known mode, defaulting to workspace.
func (p Permission) Valid() Permission {
	switch p {
	case PermissionPlan, PermissionWorkspace, PermissionFull:
		return p
	default:
		return PermissionWorkspace
	}
}

type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// ContextWindow is how many tokens the model can hold, which is what the
	// prompt bar's gauge measures the live context against. 0 means unknown
	// and the gauge shows the count without a total.
	ContextWindow int64 `json:"context_window,omitempty"`
}

// TurnRequest is one prompt and the context needed to answer it.
type TurnRequest struct {
	WorkDir string
	Prompt  string
	Model   string
	Effort  string

	// SessionID is the id we chose for the session. Providers that let the
	// caller pick one (Claude Code) use it verbatim.
	SessionID string
	// ProviderSessionID is empty on the first turn and carries the provider's
	// own id afterwards, which is what resume needs.
	ProviderSessionID string

	Permission Permission
}

type EventType string

const (
	EventSessionStarted EventType = "session_started"
	EventText           EventType = "text"
	EventThinking       EventType = "thinking"
	EventToolUse        EventType = "tool_use"
	EventToolResult     EventType = "tool_result"
	EventUsage          EventType = "usage"
	EventError          EventType = "error"
	EventDone           EventType = "done"
)

// Usage is the provider's own token accounting, never an estimate.
type Usage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`

	// ContextTokens is the size of one prompt the provider sent — cached
	// blocks included — so it can be compared against the model's window.
	// The counts above are summed over the turn; this one never is.
	ContextTokens int64 `json:"context_tokens,omitempty"`
}

type Event struct {
	Type EventType `json:"type"`
	// Text carries the delta for text/thinking, the message for error.
	Text string `json:"text,omitempty"`
	// Tool describes a tool_use or tool_result event.
	Tool *ToolEvent `json:"tool,omitempty"`
	// Usage is set on usage and done events.
	Usage *Usage `json:"usage,omitempty"`
	// ProviderSessionID is set on session_started.
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

type ToolEvent struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Input   string `json:"input,omitempty"`
	Output  string `json:"output,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// Provider runs turns for one backend.
type Provider interface {
	Name() string
	DisplayName() string
	Models() []Model
	Efforts() []string
	// Available reports whether the backend can be used right now, for
	// example whether its CLI is on PATH.
	Available() error
	// Run starts a turn and streams its events. The channel is closed when the
	// turn ends. Cancelling ctx stops the turn.
	Run(ctx context.Context, req TurnRequest) (<-chan Event, error)
}

// Registry holds the providers this build supports, in display order.
type Registry struct {
	order []Provider
	byID  map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byID: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		r.order = append(r.order, p)
		r.byID[p.Name()] = p
	}
	return r
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.byID[name]
	return p, ok
}

func (r *Registry) All() []Provider { return r.order }
