// Package agent defines the provider-neutral contract for running one agent
// turn. Implementations drive locally installed CLIs or key-based HTTP APIs. Nothing here knows about HTTP or SQL.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
)

var ErrNotImplemented = errors.New("provider not implemented")

// Permission is how much the agent may do without being asked. Anything beyond
// it is refused, or, for providers that support it, put to the user as an
// approval event (see 082-tool-approvals.md).
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
	// direct providers read their own key there. See 050-subscription-accounts.md.
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
	// EventApproval asks the user to allow or deny one tool call, or to
	// answer questions. The turn waits on it; Approval.Answer resumes it.
	EventApproval EventType = "approval"
	// EventApprovalResolved says an approval no longer waits: it was answered,
	// or the provider withdrew it. Approval carries only the ID.
	EventApprovalResolved EventType = "approval_resolved"
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
	// Approval is set on approval and approval_resolved events.
	Approval *Approval `json:"approval,omitempty"`
}

// Approval is one request a running turn waits on the user for: a tool call to
// allow or deny, or, when Questions is set, questions to answer. ID is unique
// across turns.
type Approval struct {
	ID          string `json:"id"`
	Tool        string `json:"tool,omitempty"`
	Description string `json:"description,omitempty"`
	// Input is the tool's input as JSON, shown so the user knows what runs.
	Input string `json:"input,omitempty"`
	// Questions turns the request into a form. Allowing it means answering.
	Questions []Question `json:"questions,omitempty"`
	// Allowed is set on approval_resolved when the user answered it.
	Allowed *bool `json:"allowed,omitempty"`

	answer func(Reply) error
}

// Question is one question the agent puts to the user.
type Question struct {
	// ID keys the answer. Providers without ids use the question text.
	ID       string           `json:"id"`
	Header   string           `json:"header,omitempty"`
	Question string           `json:"question"`
	Options  []QuestionOption `json:"options,omitempty"`
	// Multi allows picking more than one option.
	Multi bool `json:"multi,omitempty"`
	// Other allows a typed answer instead of, or without, options.
	Other bool `json:"other,omitempty"`
	// Secret asks the UI not to echo the typed answer.
	Secret bool `json:"secret,omitempty"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Reply is the user's answer to an Approval. Answers maps a question ID to
// the chosen option labels or typed text; it is set only for questions, and
// only when Allow is true.
type Reply struct {
	Allow   bool                `json:"allow"`
	Answers map[string][]string `json:"answers,omitempty"`
}

// NewApproval builds a request the provider answers through answer.
func NewApproval(id, tool, description, input string, answer func(Reply) error) *Approval {
	return &Approval{ID: id, Tool: tool, Description: description, Input: input, answer: answer}
}

// Answer tells the provider the user's decision. It is called at most once.
func (a *Approval) Answer(r Reply) error {
	if a.answer == nil {
		return errors.New("approval cannot be answered")
	}
	return a.answer(r)
}

// CheckReply reports whether r answers every question, with an option
// offered or, where typing is allowed, any text. A denial needs no answers.
func (a *Approval) CheckReply(r Reply) error {
	if !r.Allow {
		return nil
	}
	for _, q := range a.Questions {
		got := r.Answers[q.ID]
		if len(got) == 0 || (len(got) > 1 && !q.Multi) {
			return fmt.Errorf("question %q needs one answer", q.Question)
		}
		for _, answer := range got {
			if strings.TrimSpace(answer) == "" {
				return fmt.Errorf("question %q has an empty answer", q.Question)
			}
			if !q.Other && !slices.ContainsFunc(q.Options, func(o QuestionOption) bool { return o.Label == answer }) {
				return fmt.Errorf("question %q has no option %q", q.Question, answer)
			}
		}
	}
	return nil
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
	mu    sync.RWMutex
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byID[name]
	return p, ok
}

func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.order)
}
func (r *Registry) Add(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[p.Name()]; ok {
		return
	}
	r.order = append(r.order, p)
	r.byID[p.Name()] = p
}
