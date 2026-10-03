package store

import (
	"errors"
	"strings"
	"unicode/utf8"
)

func (s *Store) ServerName() (string, error) {
	var name string
	err := s.db.QueryRow("SELECT name FROM server_identity WHERE id = 1").Scan(&name)
	return name, err
}

func (s *Store) SetServerName(name string) error {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 3 {
		return errors.New("server name must contain at least 3 characters")
	}
	_, err := s.db.Exec("UPDATE server_identity SET name = ? WHERE id = 1", name)
	return err
}

// MachineID is a random identifier made once per data directory. Unlike the
// name and the address, it never changes, so a saved remote can be found
// again after it moves to another address.
func (s *Store) MachineID() (string, error) {
	var id string
	err := s.db.QueryRow("SELECT machine_id FROM server_identity WHERE id = 1").Scan(&id)
	return id, err
}
