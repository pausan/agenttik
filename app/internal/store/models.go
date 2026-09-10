package store

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`

	// Position is the order the Projects sidebar was dragged into. A newly
	// created project is 0, so it stays ahead of manually ordered projects.
	Position   int64 `json:"position"`
	QueueCount int64 `json:"queue_count"`

	// Filled by ListProjects: titles of the project's open sessions. Always
	// encoded, empty included, so the UI can iterate it without a guard.
	RecentSessions []SessionRef `json:"recent_sessions"`

	// Schedules are the project's open schedules, drawn above its sessions in
	// the sidebar. Same rule: always encoded, empty included.
	Schedules []Schedule `json:"schedules"`
}

type SessionRef struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	QueueCount int64  `json:"queue_count"`

	// Prompt is what the session was opened with, capped; the sidebar shows it
	// when a row is hovered. See firstPromptCol.
	Prompt string `json:"prompt,omitempty"`
}

type Session struct {
	ID                string `json:"id"`
	ProjectID         int64  `json:"project_id"`
	Title             string `json:"title"`
	Provider          string `json:"provider"`
	ProviderSessionID string `json:"provider_session_id"`
	Model             string `json:"model"`
	Effort            string `json:"effort"`
	Permission        string `json:"permission"`
	Source            string `json:"source"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	LastActiveAt      int64  `json:"last_active_at"`

	// DoneAt is 0 while the session is open and the time it was ticked off
	// otherwise. Position is the order the project view was dragged into; 0
	// means never dragged, which sorts a new session to the top.
	DoneAt     int64 `json:"done_at"`
	Position   int64 `json:"position"`
	QueueCount int64 `json:"queue_count"`

	// ScheduleID is the schedule that spawned this session, or 0 for one
	// started by hand. See 028-scheduled-jobs.md.
	ScheduleID int64 `json:"schedule_id"`

	// Denormalised for list views.
	ProjectName string `json:"project_name,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`

	// Prompt is what the session was opened with, capped; the sidebar shows it
	// when a row is hovered. See firstPromptCol.
	Prompt string `json:"prompt,omitempty"`
}

type QueuedMessage struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
	CreatedAt int64  `json:"created_at"`
}

// Schedule is a prompt plus a clock. Each time it comes due it starts a new
// session with the same prompt; it has no transcript, turns or provider thread
// of its own. See 028-scheduled-jobs.md.
type Schedule struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Title      string `json:"title"`
	Prompt     string `json:"prompt"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	Permission string `json:"permission"`

	// Every is how the next run is worked out. IntervalMinutes carries the
	// whole X hours Y minutes for EveryInterval; AtMinute is minutes past
	// local midnight for the calendar forms, whose weekday and day of the
	// month come from AnchorAt rather than from a control nobody was offered.
	Every           string `json:"every"`
	IntervalMinutes int64  `json:"interval_minutes"`
	AtMinute        int64  `json:"at_minute"`
	AnchorAt        int64  `json:"anchor_at"`

	// Remaining is how many runs are left, -1 for forever. It goes down when a
	// run finishes, never when one is skipped.
	Remaining int64 `json:"remaining"`
	Paused    bool  `json:"paused"`
	NextRunAt int64 `json:"next_run_at"`
	CreatedAt int64 `json:"created_at"`

	// DoneAt is 0 while the schedule is open and the time it was archived
	// otherwise. Position orders the sidebar, as on a session.
	DoneAt   int64 `json:"done_at"`
	Position int64 `json:"position"`

	// Running means a session this schedule started is still in flight, which
	// is what turns the next run into a skip.
	Running bool `json:"running"`

	// Denormalised for list views, as on Session.
	ProjectName string `json:"project_name,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
}

// ScheduleRun is one fire of a schedule: the session it started, or a skip.
type ScheduleRun struct {
	ID         int64  `json:"id"`
	ScheduleID int64  `json:"schedule_id"`
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at"`

	// Title is the spawned session's title, so the runs list needs no request
	// per row. Empty for a skip, which has no session.
	Title string `json:"title,omitempty"`
}

type Turn struct {
	ID               int64   `json:"id"`
	SessionID        string  `json:"session_id"`
	Model            string  `json:"model"`
	Effort           string  `json:"effort"`
	StartedAt        int64   `json:"started_at"`
	EndedAt          int64   `json:"ended_at"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Status           string  `json:"status"`
	Error            string  `json:"error"`

	// ContextTokens is the size of the last prompt sent during the turn, not a
	// sum over the turn's requests. See the migration that adds the column.
	ContextTokens int64 `json:"context_tokens"`
	// ContextWindow is the window the provider reported for this turn, or 0
	// when it reported none. See the migration that adds the column.
	ContextWindow int64 `json:"context_window"`
	// RateLimits is the subscription allowance the provider volunteered during
	// the turn, as the JSON the API serves, or empty when it volunteered none.
	RateLimits string `json:"rate_limits,omitempty"`
}

type Message struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"`
	TurnID    int64  `json:"turn_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// Stats aggregates turns: every turn in a session, or every turn of every
// session in a project.
type Stats struct {
	Turns            int64   `json:"turns"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	DurationMS       int64   `json:"duration_ms"`

	// ContextTokens is the last turn's context size, carried here so the
	// prompt bar's gauge needs no second request. It is not summed.
	ContextTokens int64 `json:"context_tokens"`
	// ContextWindow is the window the provider last reported, which the gauge
	// prefers over the static per-model figure. 0 falls back to that figure.
	ContextWindow int64 `json:"context_window"`
}

// ProjectStats is Stats over a whole project plus the session counts the
// project view shows. Sessions may run concurrently, so Running can exceed 1
// and DurationMS is summed agent time, not wall time.
type ProjectStats struct {
	Stats
	Sessions     int64 `json:"sessions"`
	Running      int64 `json:"running"`
	LastActiveAt int64 `json:"last_active_at"`
}

type Star struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	CreatedAt int64  `json:"created_at"`
}

// Session status values.
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusError   = "error"
)

// Recurrence kinds.
const (
	EveryInterval = "interval"
	EveryDay      = "day"
	EveryWeek     = "week"
	EveryMonth    = "month"
)

// Schedule run statuses. Interrupted is what a restart leaves behind: the run
// started, the process did not survive, and nobody can say it finished.
const (
	RunRunning     = "running"
	RunDone        = "done"
	RunError       = "error"
	RunInterrupted = "interrupted"
	RunSkipped     = "skipped"
)

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleThinking  = "thinking"
	RoleTool      = "tool"
	RoleError     = "error"
)
