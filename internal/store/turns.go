package store

import (
	"database/sql"
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
		    status = ?, error = ?
		 WHERE id = ?`,
		nowMillis(), t.InputTokens, t.OutputTokens, t.CacheReadTokens,
		t.CacheWriteTokens, t.CostUSD, t.Status, t.Error, t.ID)
	if err != nil {
		return fmt.Errorf("finish turn %d: %w", t.ID, err)
	}
	return nil
}

func (s *Store) SessionStats(sessionID string) (*Stats, error) {
	var st Stats
	var cost sql.NullFloat64
	var dur sql.NullInt64
	err := s.db.QueryRow(
		`SELECT COUNT(*),
		        COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		        COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_write_tokens),0),
		        SUM(cost_usd), SUM(COALESCE(ended_at, started_at) - started_at)
		 FROM turns WHERE session_id = ?`, sessionID).
		Scan(&st.Turns, &st.InputTokens, &st.OutputTokens,
			&st.CacheReadTokens, &st.CacheWriteTokens, &cost, &dur)
	if err != nil {
		return nil, fmt.Errorf("session stats %s: %w", sessionID, err)
	}
	st.CostUSD, st.DurationMS = cost.Float64, dur.Int64
	return &st, nil
}

func (s *Store) ListTurns(sessionID string) ([]Turn, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, model, effort, started_at, COALESCE(ended_at,0),
		        input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		        cost_usd, status, error
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
			&t.CacheWriteTokens, &t.CostUSD, &t.Status, &t.Error); err != nil {
			return nil, fmt.Errorf("list turns: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
