package store

import (
	"database/sql"
	"fmt"
	"slices"
)

// FlatProjectKey names a project in the flat view: its source
// ("remote/profile") and its ID there. See specs/100-flat-view.md.
type FlatProjectKey struct {
	Source string
	ID     int64
}

// FlatProject is the flat view's own place for a project. A hidden one keeps
// its visible index in Position, so restoring puts it back there.
type FlatProject struct {
	Position int
	HiddenAt int64
}

// FlatProjects reads the flat view's order and hidden set.
func (s *Store) FlatProjects() (map[FlatProjectKey]FlatProject, error) {
	rows, err := s.db.Query(`SELECT source, project_id, position, hidden_at FROM flat_projects`)
	if err != nil {
		return nil, fmt.Errorf("list flat projects: %w", err)
	}
	defer rows.Close()
	out := map[FlatProjectKey]FlatProject{}
	for rows.Next() {
		var k FlatProjectKey
		var p FlatProject
		if err := rows.Scan(&k.Source, &k.ID, &p.Position, &p.HiddenAt); err != nil {
			return nil, fmt.Errorf("list flat projects: %w", err)
		}
		out[k] = p
	}
	return out, rows.Err()
}

// SetFlatProjectOrder saves the flat view's visible order. Visible rows not
// listed are dropped: their projects are gone, or are new again.
func (s *Store) SetFlatProjectOrder(visible []FlatProjectKey) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder flat projects: %w", err)
	}
	defer tx.Rollback()
	if err := writeFlatOrder(tx, visible); err != nil {
		return err
	}
	return tx.Commit()
}

// SetFlatProjectHidden hides a project from the flat view only, or restores
// it at the index it was hidden from. visible is the flat view's order as it
// stands, which hiding or restoring saves along with the change.
func (s *Store) SetFlatProjectHidden(key FlatProjectKey, hidden bool, visible []FlatProjectKey) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("hide flat project: %w", err)
	}
	defer tx.Rollback()

	rest := make([]FlatProjectKey, 0, len(visible)+1)
	rank := 0
	for i, k := range visible {
		if k == key {
			rank = i + 1
		} else {
			rest = append(rest, k)
		}
	}
	if hidden {
		if rank == 0 {
			rank = len(rest) + 1
		}
		if err := writeFlatOrder(tx, rest); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO flat_projects (source, project_id, position, hidden_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (source, project_id) DO UPDATE SET position = excluded.position, hidden_at = excluded.hidden_at
			WHERE hidden_at = 0`, key.Source, key.ID, rank, nowMillis()); err != nil {
			return fmt.Errorf("hide flat project: %w", err)
		}
		return tx.Commit()
	}

	if rank == 0 {
		target := len(rest) + 1
		var position int
		err := tx.QueryRow(`SELECT position FROM flat_projects WHERE source = ? AND project_id = ? AND hidden_at <> 0`,
			key.Source, key.ID).Scan(&position)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("restore flat project: %w", err)
		}
		if err == nil && position >= 1 && position < target {
			target = position
		}
		rest = slices.Insert(rest, target-1, key)
	} else {
		rest = visible // already showing
	}
	if err := writeFlatOrder(tx, rest); err != nil {
		return err
	}
	return tx.Commit()
}

func writeFlatOrder(tx *sql.Tx, visible []FlatProjectKey) error {
	if _, err := tx.Exec(`DELETE FROM flat_projects WHERE hidden_at = 0`); err != nil {
		return fmt.Errorf("reorder flat projects: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO flat_projects (source, project_id, position, hidden_at)
		VALUES (?, ?, ?, 0)
		ON CONFLICT (source, project_id) DO UPDATE SET position = excluded.position, hidden_at = 0`)
	if err != nil {
		return fmt.Errorf("reorder flat projects: %w", err)
	}
	defer stmt.Close()
	for i, k := range visible {
		if _, err := stmt.Exec(k.Source, k.ID, i+1); err != nil {
			return fmt.Errorf("reorder flat projects: %w", err)
		}
	}
	return nil
}
