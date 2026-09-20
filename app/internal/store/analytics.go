package store

// AnalyticsRow aggregates turns for one task and the subscription used when
// each turn started. Project/task metadata remains current, including archives.
type AnalyticsRow struct {
	Provider         string  `json:"provider"`
	AccountID        int64   `json:"account_id"`
	ProjectID        int64   `json:"project_id"`
	ProjectName      string  `json:"project_name"`
	SessionID        string  `json:"session_id"`
	Title            string  `json:"title"`
	Turns            int64   `json:"turns"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CostTurns        int64   `json:"cost_turns"`
	InferredTurns    int64   `json:"inferred_turns"`
}

// Analytics uses a half-open interval of turn start times. One grouped query
// keeps transcripts and individual turns out of memory and off the wire.
func (s *Store) Analytics(from, to int64) ([]AnalyticsRow, error) {
	rows, err := s.db.Query(`SELECT t.provider, t.account_id, p.id, p.name, s.id, s.title,
		COUNT(*), SUM(t.input_tokens), SUM(t.output_tokens),
		SUM(t.cache_read_tokens), SUM(t.cache_write_tokens), SUM(t.cost_usd),
		SUM(t.cost_usd > 0), SUM(t.attribution_inferred)
		FROM turns t JOIN sessions s ON s.id = t.session_id
		JOIN projects p ON p.id = s.project_id
		WHERE t.started_at >= ? AND t.started_at < ?
		GROUP BY t.provider, t.account_id, p.id, s.id
		ORDER BY t.provider, t.account_id, p.id, s.id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnalyticsRow{}
	for rows.Next() {
		var row AnalyticsRow
		if err := rows.Scan(&row.Provider, &row.AccountID, &row.ProjectID, &row.ProjectName,
			&row.SessionID, &row.Title, &row.Turns, &row.InputTokens, &row.OutputTokens,
			&row.CacheReadTokens, &row.CacheWriteTokens, &row.CostUSD, &row.CostTurns,
			&row.InferredTurns); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
