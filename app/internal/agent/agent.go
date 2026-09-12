// Package agent defines the provider-neutral contract for running one agent
// turn. Implementations drive locally installed CLIs (Claude Code, Codex, GitHub Copilot) or,
// later, key-based HTTP APIs. Nothing here knows about HTTP or SQL.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	// Efforts overrides the provider-wide list for models whose supported
	// reasoning levels differ. An empty list uses Provider.Efforts instead.
	Efforts []string `json:"efforts,omitempty"`
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

	// AccountHome is the directory holding the login this turn runs on, empty
	// for the machine's own. The provider applies it the way its CLI expects;
	// nothing here reads what is inside it. See 050-subscription-accounts.md.
	AccountHome string

	// Isolated marks a one-shot request that must not inherit or persist a
	// provider conversation.
	Isolated bool

	// SessionID is the id we chose for the session. Providers that let the
	// caller pick one (Claude Code) use it verbatim.
	SessionID string
	// ProviderSessionID is empty on the first turn and carries the provider's
	// own id afterwards, which is what resume needs.
	ProviderSessionID string

	Permission Permission
}

// CheckWorkDir reports whether the turn can still be run where it was asked to
// run. Providers must call it before starting a CLI: they all set the child's
// working directory, and a failed chdir is reported by the OS against the
// binary that could not be run, so a project folder that was renamed or
// deleted otherwise surfaces as the CLI itself being missing.
func (r TurnRequest) CheckWorkDir() error {
	if r.WorkDir == "" {
		return nil
	}
	info, err := os.Stat(r.WorkDir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("work dir no longer exists: %s", r.WorkDir)
	}
	if err != nil {
		return fmt.Errorf("work dir %s: %w", r.WorkDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("work dir is not a directory: %s", r.WorkDir)
	}
	return nil
}

type EventType string

const (
	EventSessionStarted EventType = "session_started"
	EventText           EventType = "text"
	EventThinking       EventType = "thinking"
	EventToolUse        EventType = "tool_use"
	EventToolResult     EventType = "tool_result"
	EventUsage          EventType = "usage"
	EventLimits         EventType = "limits"
	EventError          EventType = "error"
	EventDone           EventType = "done"
)

// RateLimitWindow is one rolling subscription allowance window, exactly as the
// provider reports it. agenttik never estimates an allowance from token totals.
type RateLimitWindow struct {
	// Label names the window for providers that name their buckets instead of
	// sizing them. Empty means read the duration below.
	Label              string  `json:"label,omitempty"`
	UsedPercent        float64 `json:"used_percent"`
	WindowDurationMins int64   `json:"window_duration_mins,omitempty"`
	ResetsAt           int64   `json:"resets_at,omitempty"`
}

// RateLimit is one metered subscription bucket. A provider can expose two
// windows, normally a short rolling allowance and a weekly one.
type RateLimit struct {
	LimitID     string           `json:"limit_id"`
	LimitName   string           `json:"limit_name,omitempty"`
	PlanType    string           `json:"plan_type,omitempty"`
	Primary     *RateLimitWindow `json:"primary,omitempty"`
	Secondary   *RateLimitWindow `json:"secondary,omitempty"`
	ReachedType string           `json:"reached_type,omitempty"`
	// ReportedAt is when the figures were read, in Unix millis. Codex answers
	// on demand, so its reading is current; Claude Code only volunteers one
	// during a turn, so the panel says how old that reading is.
	ReportedAt int64 `json:"reported_at,omitempty"`
}

// Usage is the provider's own token accounting, never an estimate.
type Usage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`

	// ContextTokens is the size of the main agent's latest prompt — cached
	// blocks included — so it can be compared against that model's window.
	// The counts above are summed over the whole task; this one never is.
	ContextTokens int64 `json:"context_tokens,omitempty"`
	// ContextWindow is the window the provider says it ran the model in. It
	// beats the static figure in Model.ContextWindow, which is only a guess
	// until a turn reports one. 0 means the provider did not say.
	ContextWindow int64 `json:"context_window,omitempty"`

	// Providers that identify delegated work partition the task totals below.
	// UsageBreakdown says the partition is known; false keeps older turns and
	// providers without agent identity honest instead of calling all of their
	// usage "main". Any difference between the task totals and these two
	// scopes is reported as unattributed.
	MainInputTokens          int64 `json:"main_input_tokens,omitempty"`
	MainOutputTokens         int64 `json:"main_output_tokens,omitempty"`
	MainCacheReadTokens      int64 `json:"main_cache_read_tokens,omitempty"`
	MainCacheWriteTokens     int64 `json:"main_cache_write_tokens,omitempty"`
	SubagentInputTokens      int64 `json:"subagent_input_tokens,omitempty"`
	SubagentOutputTokens     int64 `json:"subagent_output_tokens,omitempty"`
	SubagentCacheReadTokens  int64 `json:"subagent_cache_read_tokens,omitempty"`
	SubagentCacheWriteTokens int64 `json:"subagent_cache_write_tokens,omitempty"`
	SubagentCount            int64 `json:"subagent_count,omitempty"`
	UsageBreakdown           bool  `json:"usage_breakdown,omitempty"`
}

type Event struct {
	Type EventType `json:"type"`
	// Text carries the delta for text/thinking, the message for error.
	Text string `json:"text,omitempty"`
	// Tool describes a tool_use or tool_result event.
	Tool *ToolEvent `json:"tool,omitempty"`
	// Usage is set on usage and done events.
	Usage *Usage `json:"usage,omitempty"`
	// Limits is set on a limits event: the subscription allowance the provider
	// volunteered mid-turn.
	Limits []RateLimit `json:"limits,omitempty"`
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

// SmallModel optionally names the provider's lightest model, which is what
// agenttik puts its own short questions to: naming a task from its first
// prompt (020-task-titles.md) and saying what a finished one came to, from its
// last reply (051-task-outcomes.md). Neither request is ever resumed as part
// of the session it is about.
type SmallModel interface {
	SmallModel() (model, effort string)
}

// Metered is the optional half of Provider for backends that can be asked for
// the signed-in account's own subscription allowance without holding its
// credentials. A provider that only volunteers one mid-turn does not implement
// it — see the limits event.
//
// home names which login to ask about, empty for the machine's own: two
// subscriptions of one provider have two allowances.
type Metered interface {
	SubscriptionLimits(ctx context.Context, home string) ([]RateLimit, error)
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
