package store

type GeneralConfig struct {
	NewItemPosition string `json:"new_item_position"`
}

func (s *Store) GetGeneralConfig() (GeneralConfig, error) {
	var c GeneralConfig
	err := s.db.QueryRow(`SELECT new_item_position FROM general_config WHERE id = 1`).Scan(&c.NewItemPosition)
	return c, err
}

func (s *Store) SetGeneralConfig(c GeneralConfig) error {
	_, err := s.db.Exec(`UPDATE general_config SET new_item_position = ? WHERE id = 1`, c.NewItemPosition)
	return err
}

// ResetPreferences changes only user preferences, never credentials or work.
func (s *Store) ResetPreferences() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`UPDATE general_config SET new_item_position = 'top' WHERE id = 1`,
		`UPDATE desktop_config SET close_to_tray = 0, toggle_shortcut = 'Ctrl+Shift+A' WHERE id = 1`,
		`DELETE FROM starred_models`,
		`DELETE FROM hidden_model_choices`,
		`UPDATE provider_accounts SET is_default = 0`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	return tx.Commit()
}
