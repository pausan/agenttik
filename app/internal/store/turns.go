package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) StartTurn(sessionID, model, effort string) (*Turn, error) {
	t := &Turn{SessionID: sessionID, Model: model, Effort: effort,
		StartedAt: nowMillis(), Status: "running"}
	res, err := s.db.Exec(
		`INSERT INTO turns (session_id, model, effort, started_at, status)
		 VALUES (?, ?, ?, ?, ?)`,
		t.SessionID, t.Model, t.Effort, t.StartedAt, t.Status)
	if err != nil {
		return nil, fmt.Errorf("start turn: %w", err)
	}
	t.ID, _ = res.LastInsertId()
	return t, nil
}

// FinishTurn writes the provider's own token accounting and closes the turn.
func (s *Store) FinishTurn(t *Turn) error {
	_, err := s.db.Exec(
		`UPDATE turns SET ended_at = ?, input_tokens = ?, output_tokens = ?,
		    cache_read_tokens = ?, cache_write_tokens = ?, cost_usd = ?,
		    context_tokens = ?, context_window = ?, rate_limits = ?,
		    status = ?, error = ?
		 WHERE id = ?`,
		nowMillis(), t.InputTokens, t.OutputTokens, t.CacheReadTokens,
		t.CacheWriteTokens, t.CostUSD, t.ContextTokens, t.ContextWindow,
		t.RateLimits, t.Status, t.Error, t.ID)
	if err != nil {
		return fmt.Errorf("finish turn %d: %w", t.ID, err)
	}
	return nil
}

// statsSelect aggregates a set of turns; the caller appends the WHERE clause.
const statsSelect = `SELECT COUNT(*),
	COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
	COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_write_tokens),0),
	SUM(cost_usd), SUM(COALESCE(ended_at, started_at) - started_at)
	FROM turns `

func (s *Store) scanStats(where string, args ...any) (*Stats, error) {
	var st Stats
	var cost sql.NullFloat64
	var dur sql.NullInt64
	err := s.db.QueryRow(statsSelect+where, args...).
		Scan(&st.Turns, &st.InputTokens, &st.OutputTokens,
			&st.CacheReadTokens, &st.CacheWriteTokens, &cost, &dur)
	if err != nil {
		return nil, err
	}
	st.CostUSD, st.DurationMS = cost.Float64, dur.Int64
	return &st, nil
}

func (s *Store) SessionStats(sessionID string) (*Stats, error) {
	st, err := s.scanStats(`WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session stats %s: %w", sessionID, err)
	}
	// The context in use is the last prompt's size, so this one is read from
	// the newest turn that reported one instead of summed. No rows yet, or a
	// provider that reports none, leaves it at zero and the gauge hides.
	var ctx sql.NullInt64
	err = s.db.QueryRow(
		`SELECT context_tokens FROM turns
		 WHERE session_id = ? AND context_tokens > 0 ORDER BY id DESC LIMIT 1`,
		sessionID).Scan(&ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session stats %s: %w", sessionID, err)
	}
	st.ContextTokens = ctx.Int64

	// The window is read the same way and from its own row: only a finished
	// turn reports one, so the newest turn with a context size often has none
	// yet. Zero leaves the gauge on the static per-model figure.
	var window sql.NullInt64
	err = s.db.QueryRow(
		`SELECT context_window FROM turns
		 WHERE session_id = ? AND context_window > 0 ORDER BY id DESC LIMIT 1`,
		sessionID).Scan(&window)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session stats %s: %w", sessionID, err)
	}
	st.ContextWindow = window.Int64
	return st, nil
}

// LatestRateLimits is the newest subscription allowance any session of the
// provider recorded, with when it was recorded. Empty means no turn has ever
// volunteered one, which is the normal state before the first turn of a run.
func (s *Store) LatestRateLimits(provider string) (string, int64, error) {
	var body string
	var at sql.NullInt64
	err := s.db.QueryRow(
		`SELECT t.rate_limits, COALESCE(t.ended_at, t.started_at)
		 FROM turns t JOIN sessions s ON s.id = t.session_id
		 WHERE s.provider = ? AND t.rate_limits <> ''
		 ORDER BY t.id DESC LIMIT 1`, provider).Scan(&body, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("latest rate limits for %s: %w", provider, err)
	}
	return body, at.Int64, nil
}

// ProjectStats aggregates every turn of every session in the project. A
// running turn counts with its elapsed time so far.
func (s *Store) ProjectStats(projectID int64) (*ProjectStats, error) {
	st, err := s.scanStats(
		`WHERE session_id IN (SELECT id FROM sessions WHERE project_id = ?)`, projectID)
	if err != nil {
		return nil, fmt.Errorf("project stats %d: %w", projectID, err)
	}
	ps := &ProjectStats{Stats: *st}
	var last sql.NullInt64
	err = s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(status = ?),0), MAX(last_active_at)
		 FROM sessions WHERE project_id = ?`, StatusRunning, projectID).
		Scan(&ps.Sessions, &ps.Running, &last)
	if err != nil {
		return nil, fmt.Errorf("project stats %d: %w", projectID, err)
	}
	ps.LastActiveAt = last.Int64
	return ps, nil
}

// ProjectDailyMetrics supplies the small activity charts on a project page.
// Sessions and turns are grouped separately, so a quiet day with only one
// kind of activity still has a row. Running turns include time spent so far.
func (s *Store) ProjectDailyMetrics(projectID int64) ([]DailyMetric, error) {
	rows, err := s.db.Query(
		`SELECT day, SUM(sessions), SUM(duration_ms) FROM (
			SELECT strftime('%Y-%m-%d', created_at / 1000, 'unixepoch') AS day,
			       COUNT(*) AS sessions, 0 AS duration_ms
			FROM sessions WHERE project_id = ? GROUP BY day
			UNION ALL
			SELECT strftime('%Y-%m-%d', t.started_at / 1000, 'unixepoch') AS day,
			       0 AS sessions, SUM(COALESCE(t.ended_at, ?) - t.started_at) AS duration_ms
			FROM turns t JOIN sessions s ON s.id = t.session_id
			WHERE s.project_id = ? GROUP BY day
		) GROUP BY day ORDER BY day`, projectID, nowMillis(), projectID)
	if err != nil {
		return nil, fmt.Errorf("project daily metrics %d: %w", projectID, err)
	}
	defer rows.Close()

	out := []DailyMetric{}
	for rows.Next() {
		var metric DailyMetric
		if err := rows.Scan(&metric.Date, &metric.Sessions, &metric.DurationMS); err != nil {
			return nil, fmt.Errorf("project daily metrics %d: %w", projectID, err)
		}
		out = append(out, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("project daily metrics %d: %w", projectID, err)
	}
	return out, nil
}

func (s *Store) ListTurns(sessionID string) ([]Turn, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, model, effort, started_at, COALESCE(ended_at,0),
		        input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		        cost_usd, context_tokens, context_window, rate_limits, status, error
		 FROM turns WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	defer rows.Close()

	out := []Turn{}
	for rows.Next() {
		var t Turn
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Model, &t.Effort, &t.StartedAt,
			&t.EndedAt, &t.InputTokens, &t.OutputTokens, &t.CacheReadTokens,
			&t.CacheWriteTokens, &t.CostUSD, &t.ContextTokens, &t.ContextWindow,
			&t.RateLimits, &t.Status, &t.Error); err != nil {
			return nil, fmt.Errorf("list turns: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
