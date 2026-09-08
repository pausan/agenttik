package store

import "fmt"

func (s *Store) AddStar(provider, model, effort string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO starred_models (provider, model, effort, created_at)
		 VALUES (?, ?, ?, ?)`, provider, model, effort, nowMillis())
	if err != nil {
		return fmt.Errorf("add star: %w", err)
	}
	return nil
}

func (s *Store) RemoveStar(provider, model, effort string) error {
	_, err := s.db.Exec(
		`DELETE FROM starred_models WHERE provider = ? AND model = ? AND effort = ?`,
		provider, model, effort)
	if err != nil {
		return fmt.Errorf("remove star: %w", err)
	}
	return nil
}

func (s *Store) ListStars() ([]Star, error) {
	rows, err := s.db.Query(
		`SELECT provider, model, effort, created_at FROM starred_models ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list stars: %w", err)
	}
	defer rows.Close()

	out := []Star{}
	for rows.Next() {
		var v Star
		if err := rows.Scan(&v.Provider, &v.Model, &v.Effort, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("list stars: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
