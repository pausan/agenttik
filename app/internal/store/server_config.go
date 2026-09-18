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
	err := s.db.QueryRow(
		`SELECT enabled, host, port, auth_enabled, password_hash, totp_secret, totp_disabled
		   FROM server_config WHERE id = 1`).
		Scan(&c.Enabled, &c.Host, &c.Port, &c.AuthEnabled, &c.PasswordHash, &c.TOTPSecret, &c.TOTPDisabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerConfig{}, nil
	}
	if err != nil {
		return ServerConfig{}, fmt.Errorf("get server config: %w", err)
	}
	return c, nil
}

// SetServerConfig replaces where the server listens, leaving the lock in
// front of it alone. The two are written separately because they are set
// separately: moving the server to another port is not a reason to ask for
// the password again, and setting a password is not a reason to rebind.
func (s *Store) SetServerConfig(c ServerConfig) error {
	_, err := s.db.Exec(
		`INSERT INTO server_config (id, enabled, host, port) VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled,
		                               host    = excluded.host,
		                               port    = excluded.port`,
		c.Enabled, c.Host, c.Port)
	if err != nil {
		return fmt.Errorf("set server config: %w", err)
	}
	return nil
}

// SetServerAuth replaces the lock on the exposed server, leaving where it
// listens alone. Credentials and the 2FA preference are written together.
func (s *Store) SetServerAuth(c ServerConfig) error {
	_, err := s.db.Exec(
		`INSERT INTO server_config (id, auth_enabled, password_hash, totp_secret, totp_disabled) VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET auth_enabled  = excluded.auth_enabled,
		                               password_hash = excluded.password_hash,
		                               totp_secret   = excluded.totp_secret,
		                               totp_disabled = excluded.totp_disabled`,
		c.AuthEnabled, c.PasswordHash, c.TOTPSecret, c.TOTPDisabled)
	if err != nil {
		return fmt.Errorf("set server auth: %w", err)
	}
	return nil
}
