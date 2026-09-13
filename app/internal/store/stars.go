package store

import "fmt"

func (s *Store) AddStar(provider string, accountID int64, model, effort string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO starred_models
		 (provider, account_id, model, effort, position, created_at)
		 VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position) + 1 FROM starred_models), 1), ?)`,
		provider, accountID, model, effort, nowMillis())
	if err != nil {
		return fmt.Errorf("add star: %w", err)
	}
	return nil
}

func (s *Store) RemoveStar(provider string, accountID int64, model, effort string) error {
	_, err := s.db.Exec(
		`DELETE FROM starred_models
		 WHERE provider = ? AND account_id = ? AND model = ? AND effort = ?`,
		provider, accountID, model, effort)
	if err != nil {
		return fmt.Errorf("remove star: %w", err)
	}
	return nil
}

func (s *Store) ListStars() ([]Star, error) {
	rows, err := s.db.Query(
		`SELECT provider, account_id, model, effort, position, created_at
		 FROM starred_models ORDER BY position, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list stars: %w", err)
	}
	defer rows.Close()

	out := []Star{}
	for rows.Next() {
		var v Star
		if err := rows.Scan(&v.Provider, &v.AccountID, &v.Model, &v.Effort, &v.Position, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("list stars: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReorderStars records the supplied combinations from first to last. Unknown
// rows are ignored, which keeps a stale Settings window from creating data.
func (s *Store) ReorderStars(stars []Star) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder stars: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE starred_models SET position = ?
		WHERE provider = ? AND account_id = ? AND model = ? AND effort = ?`)
	if err != nil {
		return fmt.Errorf("reorder stars: %w", err)
	}
	defer stmt.Close()
	for i, star := range stars {
		if _, err := stmt.Exec(i+1, star.Provider, star.AccountID, star.Model, star.Effort); err != nil {
			return fmt.Errorf("reorder stars: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reorder stars: %w", err)
	}
	return nil
}

func (s *Store) ListHiddenModelChoices() ([]HiddenModelChoice, error) {
	rows, err := s.db.Query(`SELECT provider, account_id, model
		FROM hidden_model_choices ORDER BY provider, account_id, model`)
	if err != nil {
		return nil, fmt.Errorf("list hidden model choices: %w", err)
	}
	defer rows.Close()
	out := []HiddenModelChoice{}
	for rows.Next() {
		var choice HiddenModelChoice
		if err := rows.Scan(&choice.Provider, &choice.AccountID, &choice.Model); err != nil {
			return nil, fmt.Errorf("list hidden model choices: %w", err)
		}
		out = append(out, choice)
	}
	return out, rows.Err()
}

func (s *Store) SetModelChoiceHidden(provider string, accountID int64, model string, hidden bool) error {
	var err error
	if hidden {
		_, err = s.db.Exec(`INSERT OR IGNORE INTO hidden_model_choices
			(provider, account_id, model) VALUES (?, ?, ?)`, provider, accountID, model)
	} else {
		_, err = s.db.Exec(`DELETE FROM hidden_model_choices
			WHERE provider = ? AND account_id = ? AND model = ?`, provider, accountID, model)
	}
	if err != nil {
		return fmt.Errorf("set model choice visibility: %w", err)
	}
	return nil
}
