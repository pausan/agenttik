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
