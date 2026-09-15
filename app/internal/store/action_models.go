package store

// ActionModel is a saved subscription, model and effort for one automatic action.
// Missing rows use the action's default, so new actions need no schema change.
type ActionModel struct {
	Provider  string `json:"provider"`
	AccountID int64  `json:"account_id"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
}

func (s *Store) ActionModels() (map[string]ActionModel, error) {
	rows, err := s.db.Query(`SELECT action, provider, account_id, model, effort FROM action_models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]ActionModel{}
	for rows.Next() {
		var action string
		var m ActionModel
		if err := rows.Scan(&action, &m.Provider, &m.AccountID, &m.Model, &m.Effort); err != nil {
			return nil, err
		}
		result[action] = m
	}
	return result, rows.Err()
}

func (s *Store) SetActionModel(action string, m *ActionModel) error {
	if m == nil {
		_, err := s.db.Exec(`DELETE FROM action_models WHERE action = ?`, action)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO action_models(action, provider, account_id, model, effort) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(action) DO UPDATE SET provider=excluded.provider, account_id=excluded.account_id, model=excluded.model, effort=excluded.effort`,
		action, m.Provider, m.AccountID, m.Model, m.Effort)
	return err
}
