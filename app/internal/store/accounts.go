package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// One machine can hold more than one subscription for the same provider — a
// company Claude account and a personal one — because each CLI keeps its
// login in a directory that can be pointed elsewhere. A row here is an alias
// and that directory. What is inside it stays the CLI's business: agenttik
// never reads, copies or forwards a credential.
//
// Account 0 is not a row. It is the machine's own signed-in CLI, which is what
// every task ran on before this table existed and still what a task runs on
// when nothing else is chosen. See 050-subscription-accounts.md.

// SystemAccount is the id of the CLI's own login, the account a task has
// unless it was given another.
const SystemAccount int64 = 0

const accountCols = `id, provider, alias, home, is_default, created_at`

func scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	v := &Account{}
	err := row.Scan(&v.ID, &v.Provider, &v.Alias, &v.Home, &v.IsDefault, &v.CreatedAt)
	return v, err
}

// ListAccounts returns every configured subscription, in the order they were
// added, grouped by provider. The system account is not among them: it needs
// no row and has nothing to store.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountCols +
		` FROM provider_accounts ORDER BY provider, created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	out := []Account{}
	for rows.Next() {
		v, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// GetAccount returns one subscription. A missing one is ErrNotFound rather
// than the system account: a task whose subscription was removed must say so,
// not quietly run on someone else's allowance.
func (s *Store) GetAccount(id int64) (*Account, error) {
	v, err := scanAccount(s.db.QueryRow(
		`SELECT `+accountCols+` FROM provider_accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account %d: %w", id, err)
	}
	return v, nil
}

// CreateAccount adds a subscription. The alias is unique per provider, since
// it is how the two are told apart in every picker that offers them.
func (s *Store) CreateAccount(v *Account) error {
	v.CreatedAt = nowMillis()
	res, err := s.db.Exec(
		`INSERT INTO provider_accounts (provider, alias, home, is_default, created_at)
		 VALUES (?, ?, ?, ?, ?)`, v.Provider, v.Alias, v.Home, v.IsDefault, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	if v.IsDefault {
		return s.SetDefaultAccount(v.Provider, v.ID)
	}
	return nil
}

// UpdateAccount renames a subscription or repoints it at another directory.
func (s *Store) UpdateAccount(v *Account) error {
	res, err := s.db.Exec(
		`UPDATE provider_accounts SET alias = ?, home = ? WHERE id = ?`,
		v.Alias, v.Home, v.ID)
	if err != nil {
		return fmt.Errorf("update account %d: %w", v.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAccount forgets a subscription. The directory it named is left alone:
// it holds a login this app did not create and must not destroy.
func (s *Store) DeleteAccount(id int64) error {
	res, err := s.db.Exec(`DELETE FROM provider_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDefaultAccount chooses what a new task on this provider starts on.
// SystemAccount clears the flag from every row instead of setting one, since
// the CLI's own login has no row to carry it.
func (s *Store) SetDefaultAccount(provider string, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("set default account: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE provider_accounts SET is_default = 0 WHERE provider = ?`, provider); err != nil {
		return fmt.Errorf("set default account: %w", err)
	}
	if id != SystemAccount {
		res, err := tx.Exec(
			`UPDATE provider_accounts SET is_default = 1 WHERE id = ? AND provider = ?`,
			id, provider)
		if err != nil {
			return fmt.Errorf("set default account: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set default account: %w", err)
	}
	return nil
}

// DefaultAccount is the subscription a new task on this provider starts on.
// No flagged row means the CLI's own login.
func (s *Store) DefaultAccount(provider string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM provider_accounts WHERE provider = ? AND is_default = 1`, provider).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return SystemAccount, nil
	}
	if err != nil {
		return SystemAccount, fmt.Errorf("default account for %s: %w", provider, err)
	}
	return id, nil
}

// AccountUsage counts what still points at a subscription, which is what the
// confirmation before removing one says out loud. Tasks and jobs are counted
// separately because they read differently on screen.
func (s *Store) AccountUsage(id int64) (sessions, schedules int64, err error) {
	if id == SystemAccount {
		return 0, 0, nil
	}
	if err = s.db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE account_id = ?`, id).Scan(&sessions); err != nil {
		return 0, 0, fmt.Errorf("account %d usage: %w", id, err)
	}
	if err = s.db.QueryRow(
		`SELECT COUNT(*) FROM schedules WHERE account_id = ?`, id).Scan(&schedules); err != nil {
		return 0, 0, fmt.Errorf("account %d usage: %w", id, err)
	}
	return sessions, schedules, nil
}
