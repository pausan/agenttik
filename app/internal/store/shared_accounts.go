package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShareAccounts imports legacy profile subscriptions before its runner starts.
// Both the imported rows and local references are committed together. Models,
// favourites and all other settings continue to use this profile's database.
func (s *Store) ShareAccounts(root *Store) error {
	if s.accounts != nil {
		return nil
	}
	var migrated bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM shared_accounts_migrated)`).Scan(&migrated); err != nil {
		return err
	}
	if migrated {
		s.accounts = root
		return nil
	}
	rows, err := s.ListAccounts()
	if err != nil {
		return err
	}
	// Move managed logins out of the removable profile directory. A restart after
	// the rename but before the transaction can reuse the same destination.
	oldHome := filepath.Join(s.Dir(), "accounts")
	newHome := filepath.Join(root.Dir(), "accounts", "profiles", filepath.Base(s.Dir()))
	if _, err := os.Stat(oldHome); err == nil {
		if err := os.MkdirAll(filepath.Dir(newHome), 0700); err != nil {
			return err
		}
		if err := os.Rename(oldHome, newHome); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := s.db.Exec(`ATTACH DATABASE ? AS shared_accounts`, root.Path()); err != nil {
		return err
	}
	defer s.db.Exec(`DETACH DATABASE shared_accounts`)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tables := []string{"sessions", "schedules", "queued_messages", "starred_models", "hidden_model_choices", "action_models"}
	// Deleted legacy accounts must remain unresolved, even when the root uses
	// their old IDs. Negative IDs can never name a real subscription.
	for _, table := range tables {
		if _, err := tx.Exec(`UPDATE ` + table + ` SET account_id = -4611686018427387904 - account_id WHERE account_id > 0 AND account_id NOT IN (SELECT id FROM provider_accounts)`); err != nil {
			return err
		}
	}

	for _, row := range rows {
		home := row.Home
		if rel, err := filepath.Rel(oldHome, home); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			home = filepath.Join(newHome, rel)
		}
		alias := row.Alias
		for suffix := 1; ; suffix++ {
			var count int
			if err := tx.QueryRow(`SELECT count(*) FROM shared_accounts.provider_accounts WHERE provider = ? AND alias = ?`, row.Provider, alias).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				break
			}
			alias = fmt.Sprintf("%s (%d)", row.Alias, suffix)
		}
		result, err := tx.Exec(`INSERT INTO shared_accounts.provider_accounts(provider, alias, home, is_default, created_at) VALUES (?, ?, ?, 0, ?)`, row.Provider, alias, home, row.CreatedAt)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		// Negative temporary IDs prevent collisions with other old local IDs.
		for _, table := range tables {
			if _, err := tx.Exec(`UPDATE `+table+` SET account_id = ? WHERE account_id = ?`, -id, row.ID); err != nil {
				return err
			}
		}
	}
	for _, table := range tables {
		if _, err := tx.Exec(`UPDATE ` + table + ` SET account_id = -account_id WHERE account_id < 0 AND account_id > -4611686018427387904`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM provider_accounts`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO shared_accounts_migrated(id) VALUES (1)`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.accounts = root
	return nil
}
