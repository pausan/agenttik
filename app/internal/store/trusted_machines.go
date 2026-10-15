package store

import "fmt"

// TrustedMachine is another agenttik allowed to sign in to the exposed server
// with its key, because it once signed in with the password.
type TrustedMachine struct {
	Key        string `json:"key"`
	MachineID  string `json:"machine_id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	AddedAt    int64  `json:"added_at"`
	LastSeenAt int64  `json:"last_seen_at"`
}

// TrustMachine adds a machine, or refreshes the one with the same key while
// keeping when it was first added.
func (s *Store) TrustMachine(m TrustedMachine) error {
	now := nowMillis()
	_, err := s.db.Exec(
		`INSERT INTO trusted_machines (public_key, machine_id, name, address, added_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(public_key) DO UPDATE SET machine_id   = excluded.machine_id,
		                                       name         = excluded.name,
		                                       address      = excluded.address,
		                                       last_seen_at = excluded.last_seen_at`,
		m.Key, m.MachineID, m.Name, m.Address, now, now)
	if err != nil {
		return fmt.Errorf("trust machine: %w", err)
	}
	return nil
}

// VisitTrustedMachine records a key sign-in and reports whether the key is
// trusted. Unknown keys change nothing.
func (s *Store) VisitTrustedMachine(m TrustedMachine) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE trusted_machines SET machine_id = ?, name = ?, address = ?, last_seen_at = ? WHERE public_key = ?`,
		m.MachineID, m.Name, m.Address, nowMillis(), m.Key)
	if err != nil {
		return false, fmt.Errorf("visit trusted machine: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// TrustedMachines lists trusted machines, most recently seen first.
func (s *Store) TrustedMachines() ([]TrustedMachine, error) {
	rows, err := s.db.Query(
		`SELECT public_key, machine_id, name, address, added_at, last_seen_at
		   FROM trusted_machines ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list trusted machines: %w", err)
	}
	defer rows.Close()
	out := []TrustedMachine{}
	for rows.Next() {
		var m TrustedMachine
		if err := rows.Scan(&m.Key, &m.MachineID, &m.Name, &m.Address, &m.AddedAt, &m.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ForgetTrustedMachine removes a machine, reporting whether it was there.
func (s *Store) ForgetTrustedMachine(key string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM trusted_machines WHERE public_key = ?`, key)
	if err != nil {
		return false, fmt.Errorf("forget trusted machine: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
