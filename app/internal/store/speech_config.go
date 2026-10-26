package store

type SpeechConfig struct {
	Model   string `json:"model"`
	Correct bool   `json:"correct"`
}

func (s *Store) GetSpeechConfig() (SpeechConfig, error) {
	var c SpeechConfig
	err := s.db.QueryRow(`SELECT model, correct FROM speech_config WHERE id = 1`).Scan(&c.Model, &c.Correct)
	return c, err
}

func (s *Store) SetSpeechConfig(c SpeechConfig) error {
	_, err := s.db.Exec(`UPDATE speech_config SET model = ?, correct = ? WHERE id = 1`, c.Model, c.Correct)
	return err
}
