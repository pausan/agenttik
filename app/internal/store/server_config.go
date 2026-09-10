package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// GetServerConfig returns the saved exposed-server setting, or the zero
// value if it has never been set — the caller fills that in with the app's
// compiled-in default rather than a second copy of it living here.
func (s *Store) GetServerConfig() (ServerConfig, error) {
	var c ServerConfig
	err := s.db.QueryRow(`SELECT enabled, host, port FROM server_config WHERE id = 1`).
		Scan(&c.Enabled, &c.Host, &c.Port)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerConfig{}, nil
	}
	if err != nil {
		return ServerConfig{}, fmt.Errorf("get server config: %w", err)
	}
	return c, nil
}

// SetServerConfig replaces the saved exposed-server setting. There is only
// ever one row, so REPLACE is simpler than an upsert and nothing references
// it to make deleting and reinserting a problem.
func (s *Store) SetServerConfig(c ServerConfig) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO server_config (id, enabled, host, port) VALUES (1, ?, ?, ?)`,
		c.Enabled, c.Host, c.Port)
	if err != nil {
		return fmt.Errorf("set server config: %w", err)
	}
	return nil
}
