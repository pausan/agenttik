package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		s.Close()
	}
}

func TestProjectWithOpenSessions(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("agenttik", "/home/u/agenttik")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// The sidebar keeps every open session so each can be reordered there.
	for i := 0; i < 6; i++ {
		sess := &Session{ID: string(rune('a' + i)), ProjectID: p.ID,
			Title: "session", Provider: "claude", Model: "opus"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if got := len(projects[0].RecentSessions); got != 6 {
		t.Errorf("got %d open sessions, want 6", got)
	}
}

func TestReorderProjectsDrivesSidebarOrder(t *testing.T) {
	s := testStore(t)
	a, _ := s.CreateProject("alpha", "/tmp/alpha")
	b, _ := s.CreateProject("beta", "/tmp/beta")
	c, _ := s.CreateProject("charlie", "/tmp/charlie")

	must(t, s.ReorderProjects([]int64{c.ID, a.ID, b.ID}))
	projects, err := s.ListProjects()
	must(t, err)
	if got := []int64{projects[0].ID, projects[1].ID, projects[2].ID}; got[0] != c.ID || got[1] != a.ID || got[2] != b.ID {
		t.Errorf("project order = %v, want [%d %d %d]", got, c.ID, a.ID, b.ID)
	}
}

func TestListSessionsFilters(t *testing.T) {
	s := testStore(t)
	p1, _ := s.CreateProject("alpha", "/tmp/alpha")
	p2, _ := s.CreateProject("beta", "/tmp/beta")
	must(t, s.CreateSession(&Session{ID: "s1", ProjectID: p1.ID, Title: "fix login", Provider: "claude", Model: "opus"}))
	must(t, s.CreateSession(&Session{ID: "s2", ProjectID: p2.ID, Title: "add tests", Provider: "claude", Model: "opus"}))

	all, err := s.ListSessions(SessionFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all: got %d sessions, err %v", len(all), err)
	}
	if all[0].ProjectName == "" {
		t.Error("project name not joined into session row")
	}

	byProject, _ := s.ListSessions(SessionFilter{ProjectID: p2.ID})
	if len(byProject) != 1 || byProject[0].ID != "s2" {
		t.Errorf("project filter: got %+v", byProject)
	}

	byTitle, _ := s.ListSessions(SessionFilter{Query: "login"})
	if len(byTitle) != 1 || byTitle[0].ID != "s1" {
		t.Errorf("title filter: got %+v", byTitle)
	}

	byPath, _ := s.ListSessions(SessionFilter{Query: "/tmp/beta"})
	if len(byPath) != 1 || byPath[0].ID != "s2" {
		t.Errorf("path filter: got %+v", byPath)
	}
}

func TestRunningSessionsIgnoreWindow(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	must(t, s.CreateSession(&Session{ID: "old", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	must(t, s.CreateSession(&Session{ID: "run", ProjectID: p.ID, Provider: "claude", Model: "opus", Status: StatusRunning}))

	// A window in the future excludes everything except the running session.
	got, err := s.ListSessions(SessionFilter{Since: nowMillis() + 60_000})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != "run" {
		t.Errorf("got %+v, want only the running session", got)
	}
}

func TestTurnMetricsAggregate(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	must(t, s.CreateSession(&Session{ID: "s1", ProjectID: p.ID, Provider: "claude", Model: "opus"}))

	for i := 0; i < 2; i++ {
		turn, err := s.StartTurn("s1", "opus", "high")
		if err != nil {
			t.Fatalf("start turn: %v", err)
		}
		turn.InputTokens, turn.OutputTokens, turn.CostUSD = 100, 20, 0.5
		turn.Status = "ok"
		must(t, s.FinishTurn(turn))
	}

	st, err := s.SessionStats("s1")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.Turns != 2 || st.InputTokens != 200 || st.OutputTokens != 40 {
		t.Errorf("got %+v", st)
	}
	if st.CostUSD != 1.0 {
		t.Errorf("cost = %v, want 1.0", st.CostUSD)
	}
}

func TestStarsRoundTrip(t *testing.T) {
	s := testStore(t)
	must(t, s.AddStar("claude", "opus", "high"))
	must(t, s.AddStar("claude", "opus", "high")) // duplicate is a no-op
	stars, _ := s.ListStars()
	if len(stars) != 1 {
		t.Fatalf("got %d stars, want 1", len(stars))
	}
	must(t, s.RemoveStar("claude", "opus", "high"))
	stars, _ = s.ListStars()
	if len(stars) != 0 {
		t.Errorf("got %d stars after remove, want 0", len(stars))
	}
}

// TestLastReplyIsTheAgentsClosingProse covers what a finished task is
// summarised from: the newest assistant row, ignoring the user's prompts, the
// tool calls between them and any error the turn ended on.
func TestLastReplyIsTheAgentsClosingProse(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("alpha", "/tmp/alpha")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sess := &Session{ID: "s1", ProjectID: p.ID, Provider: "claude", Model: "opus"}
	must(t, s.CreateSession(sess))

	// A task that has not run yet is blank rather than an error: nothing to
	// summarise is the ordinary case, not a failure.
	if reply, err := s.LastReply("s1"); err != nil || reply != "" {
		t.Errorf("LastReply of a session with no messages = %q, %v; want blank", reply, err)
	}

	for _, m := range []struct{ role, content string }{
		{RoleUser, "fix the flaky test"},
		{RoleAssistant, "Looking at the scheduler now."},
		{RoleTool, "Edit app/internal/runner/schedules.go"},
		{RoleAssistant, "Done — the sleep is gone and the test passes."},
		{RoleTool, "Bash go test ./..."},
	} {
		if _, err := s.AddMessage("s1", 0, m.role, m.content); err != nil {
			t.Fatalf("add %s message: %v", m.role, err)
		}
	}
	reply, err := s.LastReply("s1")
	if err != nil {
		t.Fatalf("last reply: %v", err)
	}
	if reply != "Done — the sleep is gone and the test passes." {
		t.Errorf("LastReply = %q", reply)
	}

	// Long replies are capped rather than refused: the question is charged for
	// every time a task is archived.
	long := strings.Repeat("a", 5000)
	if _, err := s.AddMessage("s1", 0, RoleAssistant, long); err != nil {
		t.Fatalf("add long reply: %v", err)
	}
	reply, err = s.LastReply("s1")
	if err != nil {
		t.Fatalf("last reply: %v", err)
	}
	if len(reply) != 4000 {
		t.Errorf("LastReply length = %d, want the 4000-character cap", len(reply))
	}
}

// TestSummaryIsWrittenWithoutTouchingActivity guards the one rule the column
// shares with archiving: a summary is written about a task, not by it, so the
// clock the archive is ordered by must not move.
func TestSummaryIsWrittenWithoutTouchingActivity(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("alpha", "/tmp/alpha")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	must(t, s.CreateSession(&Session{ID: "s1", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	before, err := s.GetSession("s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	must(t, s.SetSessionSummary("s1", "Removed the sleep from the scheduler test"))
	after, err := s.GetSession("s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if after.Summary != "Removed the sleep from the scheduler test" {
		t.Errorf("summary = %q", after.Summary)
	}
	if after.LastActiveAt != before.LastActiveAt {
		t.Errorf("last_active_at moved: %d then %d", before.LastActiveAt, after.LastActiveAt)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Project stats span every session in the project, including one still
// running, and ignore other projects.
func TestProjectStatsSpanSessions(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	other, _ := s.CreateProject("beta", "/tmp/beta")
	must(t, s.CreateSession(&Session{ID: "a1", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	must(t, s.CreateSession(&Session{ID: "a2", ProjectID: p.ID, Provider: "claude", Model: "opus", Status: StatusRunning}))
	must(t, s.CreateSession(&Session{ID: "b1", ProjectID: other.ID, Provider: "claude", Model: "opus"}))

	for _, id := range []string{"a1", "a2", "b1"} {
		turn, err := s.StartTurn(id, "opus", "")
		if err != nil {
			t.Fatalf("start turn: %v", err)
		}
		turn.InputTokens, turn.CostUSD, turn.Status = 10, 0.25, "ok"
		must(t, s.FinishTurn(turn))
	}
	if _, err := s.StartTurn("a2", "opus", ""); err != nil { // still running
		t.Fatalf("start turn: %v", err)
	}

	st, err := s.ProjectStats(p.ID)
	if err != nil {
		t.Fatalf("project stats: %v", err)
	}
	if st.Sessions != 2 || st.Running != 1 {
		t.Errorf("sessions = %d running = %d, want 2 and 1", st.Sessions, st.Running)
	}
	if st.Turns != 3 || st.InputTokens != 20 || st.CostUSD != 0.5 {
		t.Errorf("stats = %+v, want 3 turns, 20 input tokens, cost 0.5", st.Stats)
	}
	if st.LastActiveAt == 0 {
		t.Error("last_active_at not set")
	}

	empty, _ := s.ProjectStats(other.ID + 1)
	if empty.Sessions != 0 || empty.Turns != 0 {
		t.Errorf("unknown project stats = %+v, want zeros", empty)
	}
}

func TestSetProjectName(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	must(t, s.SetProjectName(p.ID, "renamed"))
	got, _ := s.GetProject(p.ID)
	if got.Name != "renamed" {
		t.Errorf("name = %q, want renamed", got.Name)
	}
	if err := s.SetProjectName(p.ID+1, "x"); err != ErrNotFound {
		t.Errorf("rename missing project err = %v, want ErrNotFound", err)
	}
}

func TestDoneSessionsLeaveProjectViewsOnly(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	must(t, s.CreateSession(&Session{ID: "open", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	must(t, s.CreateSession(&Session{ID: "shut", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	must(t, s.SetSessionDone("shut", true))

	open, err := s.ListSessions(SessionFilter{ProjectID: p.ID, ExcludeDone: true})
	must(t, err)
	if len(open) != 1 || open[0].ID != "open" {
		t.Errorf("project view = %+v, want only the open session", open)
	}
	all, err := s.ListSessions(SessionFilter{})
	must(t, err)
	if len(all) != 2 {
		t.Errorf("Sessions list = %d rows, want 2: done sessions stay", len(all))
	}
	projects, err := s.ListProjects()
	must(t, err)
	if got := projects[0].RecentSessions; len(got) != 1 || got[0].ID != "open" {
		t.Errorf("sidebar = %+v, want only the open session", got)
	}

	must(t, s.SetSessionDone("shut", false))
	open, err = s.ListSessions(SessionFilter{ProjectID: p.ID, ExcludeDone: true})
	must(t, err)
	if len(open) != 2 {
		t.Errorf("unticking should bring it back, got %d rows", len(open))
	}
}

func TestReorderSessionsDrivesProjectOrder(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	other, _ := s.CreateProject("beta", "/tmp/beta")
	for _, id := range []string{"a", "b", "c"} {
		must(t, s.CreateSession(&Session{ID: id, ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	}
	must(t, s.CreateSession(&Session{ID: "z", ProjectID: other.ID, Provider: "claude", Model: "opus"}))

	// "z" belongs to another project and must not be moved into this one.
	must(t, s.ReorderSessions(p.ID, []string{"c", "a", "b", "z"}))
	got, err := s.ListSessions(SessionFilter{ProjectID: p.ID})
	must(t, err)
	if len(got) != 3 || got[0].ID != "c" || got[1].ID != "a" || got[2].ID != "b" {
		t.Fatalf("order = %+v, want c a b", ids(got))
	}
	if z, err := s.GetSession("z"); err != nil || z.Position != 0 {
		t.Errorf("session in another project was moved: %+v %v", z, err)
	}

	// A session created after a drag is placed after every existing session.
	must(t, s.CreateSession(&Session{ID: "new", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	got, err = s.ListSessions(SessionFilter{ProjectID: p.ID})
	must(t, err)
	if got[len(got)-1].ID != "new" {
		t.Errorf("order = %+v, want the new session last", ids(got))
	}
}

func TestContextTokensComeFromTheLastTurn(t *testing.T) {
	s := testStore(t)
	p, _ := s.CreateProject("alpha", "/tmp/alpha")
	must(t, s.CreateSession(&Session{ID: "s1", ProjectID: p.ID, Provider: "claude", Model: "opus"}))
	for _, n := range []int64{40_000, 90_000} {
		turn, err := s.StartTurn("s1", "opus", "")
		must(t, err)
		turn.ContextTokens, turn.InputTokens, turn.Status = n, 10, "ok"
		must(t, s.FinishTurn(turn))
	}
	stats, err := s.SessionStats("s1")
	must(t, err)
	if stats.ContextTokens != 90_000 {
		t.Errorf("context tokens = %d, want the last turn's 90000, not a sum", stats.ContextTokens)
	}
	if stats.InputTokens != 20 {
		t.Errorf("input tokens = %d, want the sum 20", stats.InputTokens)
	}
}

func ids(sessions []Session) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID
	}
	return out
}

// TestServerConfigStartsEmpty checks the "never saved" answer the caller
// fills in with the app's own default, and that no lock comes turned on.
func TestServerConfigStartsEmpty(t *testing.T) {
	s := testStore(t)
	got, err := s.GetServerConfig()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != (ServerConfig{}) {
		t.Errorf("GetServerConfig() = %+v, want the zero value", got)
	}
}

// TestServerConfigAndAuthAreWrittenApart is the property the two writers
// exist for: they share one row, and neither may clear the other's columns.
// Moving the server to a new port must not drop the password, and setting a
// password must not move the server.
func TestServerConfigAndAuthAreWrittenApart(t *testing.T) {
	s := testStore(t)
	if err := s.SetServerConfig(ServerConfig{Enabled: true, Host: "0.0.0.0", Port: 7717}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	auth := ServerConfig{AuthEnabled: true, PasswordHash: "$2a$10$hash", TOTPSecret: "GEZDGNBVGY3TQOJQ"}
	if err := s.SetServerAuth(auth); err != nil {
		t.Fatalf("set auth: %v", err)
	}

	// The lock survives a move.
	if err := s.SetServerConfig(ServerConfig{Enabled: true, Host: "127.0.0.1", Port: 9000}); err != nil {
		t.Fatalf("move: %v", err)
	}
	got, err := s.GetServerConfig()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "127.0.0.1" || got.Port != 9000 {
		t.Errorf("address = %s:%d, want 127.0.0.1:9000", got.Host, got.Port)
	}
	if !got.AuthEnabled || got.PasswordHash != auth.PasswordHash || got.TOTPSecret != auth.TOTPSecret {
		t.Errorf("the lock was lost moving the server: %+v", got)
	}

	// And the address survives a new password.
	if err := s.SetServerAuth(ServerConfig{AuthEnabled: true, PasswordHash: "$2a$10$other", TOTPSecret: auth.TOTPSecret}); err != nil {
		t.Fatalf("change password: %v", err)
	}
	got, err = s.GetServerConfig()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Enabled || got.Host != "127.0.0.1" || got.Port != 9000 {
		t.Errorf("the address was lost changing the password: %+v", got)
	}
	if got.PasswordHash != "$2a$10$other" {
		t.Errorf("PasswordHash = %q, want the new one", got.PasswordHash)
	}
}

// TestSetServerAuthOnAFreshRow checks the lock can be the first thing ever
// written, which is what happens when somebody turns it on before ever
// touching the host or the port.
func TestSetServerAuthOnAFreshRow(t *testing.T) {
	s := testStore(t)
	if err := s.SetServerAuth(ServerConfig{AuthEnabled: true, PasswordHash: "h", TOTPSecret: "GEZDGNBVGY3TQOJQ"}); err != nil {
		t.Fatalf("set auth: %v", err)
	}
	got, err := s.GetServerConfig()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.AuthEnabled || got.Host != "" || got.Port != 0 {
		t.Errorf("got %+v, want the lock set and the address still unsaved", got)
	}
}
