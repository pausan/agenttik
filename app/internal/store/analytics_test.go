package store

import "testing"

func TestAnalyticsWindowAndAttribution(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("Alpha", "/tmp/analytics-alpha")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "task", ProjectID: p.ID, Provider: "claude", AccountID: 4, Model: "model"}))
	add := func(at int64, cost float64, input int64) {
		t.Helper()
		turn, err := s.StartTurn("task", "model", "")
		must(t, err)
		turn.CostUSD, turn.InputTokens, turn.OutputTokens = cost, input, 3
		turn.CacheReadTokens, turn.CacheWriteTokens, turn.Status = 5, 2, "ok"
		must(t, s.FinishTurn(turn))
		_, err = s.db.Exec(`UPDATE turns SET started_at = ? WHERE id = ?`, at, turn.ID)
		must(t, err)
	}
	add(99, 100, 1000) // before the interval
	add(100, 1.25, 10) // inclusive lower bound
	add(120, 0.25, 20)
	must(t, s.SetSessionModel("task", "codex", 9, "other", "", true))
	add(150, 0, 40)
	add(200, 200, 2000) // exclusive upper bound
	// A later choice must not relabel earlier turns, even within one provider.
	must(t, s.SetSessionModel("task", "codex", 10, "other", "", false))
	add(170, 0, 8)
	_, err = s.db.Exec(`UPDATE sessions SET done_at = 1 WHERE id = 'task'`)
	must(t, err)
	must(t, s.SetProjectHidden(p.ID, true))
	rows, err := s.Analytics(100, 200)
	must(t, err)
	if len(rows) != 3 {
		t.Fatalf("got %+v, want three provider/subscription rows", rows)
	}
	claude := rows[0]
	if claude.Provider != "claude" || claude.AccountID != 4 || claude.Turns != 2 || claude.CostUSD != 1.5 || claude.InputTokens != 30 || claude.OutputTokens != 6 || claude.CacheReadTokens != 10 || claude.CacheWriteTokens != 4 || claude.CostTurns != 2 || claude.InferredTurns != 0 {
		t.Fatalf("wrong Claude totals: %+v", claude)
	}
	if rows[1].AccountID != 9 || rows[1].CostTurns != 0 || rows[1].InputTokens != 40 || rows[2].AccountID != 10 {
		t.Fatalf("wrong subscription attribution: %+v", rows)
	}
	empty, err := s.Analytics(201, 300)
	must(t, err)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("expected empty array, got %+v", empty)
	}
}

func TestAnalyticsMigrationBackfillsLegacyTurns(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("Legacy", "/tmp/analytics-legacy")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "old", ProjectID: p.ID, Provider: "claude", AccountID: 7, Model: "m"}))
	_, err = s.db.Exec(`INSERT INTO turns (session_id, model, started_at) VALUES ('old', 'm', 1)`)
	must(t, err)
	// Recreate the schema just before the analytics migration.
	for _, query := range []string{
		`DROP INDEX idx_turns_started`,
		`ALTER TABLE turns DROP COLUMN provider`,
		`ALTER TABLE turns DROP COLUMN account_id`,
		`ALTER TABLE turns DROP COLUMN attribution_inferred`,
	} {
		_, err = s.db.Exec(query)
		must(t, err)
	}
	must(t, applyMigrations(s.db, len(migrations)-1, migrations))
	must(t, s.SetSessionModel("old", "codex", 0, "m", "", true))
	rows, err := s.Analytics(0, 2)
	must(t, err)
	if len(rows) != 1 || rows[0].Provider != "claude" || rows[0].AccountID != 7 || rows[0].InferredTurns != 1 {
		t.Fatalf("legacy attribution was not preserved: %+v", rows)
	}
}
