package store

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`

	// Filled by ListProjects: titles of the most recent sessions.
	RecentSessions []SessionRef `json:"recent_sessions,omitempty"`
}

type SessionRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
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

	// Denormalised for list views.
	ProjectName string `json:"project_name,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
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
}

type Message struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"`
	TurnID    int64  `json:"turn_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// Stats aggregates every turn in a session.
type Stats struct {
	Turns            int64   `json:"turns"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	DurationMS       int64   `json:"duration_ms"`
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

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleThinking  = "thinking"
	RoleTool      = "tool"
	RoleError     = "error"
)
