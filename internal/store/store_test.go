package store

import (
	"path/filepath"
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

func TestProjectWithRecentSessions(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("agenttik", "/home/u/agenttik")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// Six sessions, but the sidebar only shows five.
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
	if got := len(projects[0].RecentSessions); got != recentSessionsPerProject {
		t.Errorf("got %d recent sessions, want %d", got, recentSessionsPerProject)
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

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
