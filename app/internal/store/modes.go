package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrModeTitleTaken is returned when another mode already has the title. Titles
// are what a mode is picked by, in the selectors and as a Claude subagent, so
// two modes called the same could not be told apart.
var ErrModeTitleTaken = errors.New("another mode already has that title")

// What a task's or a schedule's ModeID can hold besides a mode's own ID: the
// project's mode, or none at all. A project's ModeID is a mode or 0 for none.
// See 101-modes.md.
const (
	ModeOfProject int64 = 0
	ModeNone      int64 = -1
)

// ListModes returns every mode, by title.
func (s *Store) ListModes() ([]Mode, error) {
	rows, err := s.db.Query(`SELECT id, title, body, created_at FROM modes ORDER BY title COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list modes: %w", err)
	}
	defer rows.Close()
	modes := []Mode{}
	for rows.Next() {
		var m Mode
		if err := rows.Scan(&m.ID, &m.Title, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("list modes: %w", err)
		}
		modes = append(modes, m)
	}
	return modes, rows.Err()
}

func (s *Store) GetMode(id int64) (*Mode, error) {
	var m Mode
	err := s.db.QueryRow(`SELECT id, title, body, created_at FROM modes WHERE id = ?`, id).
		Scan(&m.ID, &m.Title, &m.Body, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mode %d: %w", id, err)
	}
	return &m, nil
}

func (s *Store) CreateMode(title, body string) (*Mode, error) {
	m := &Mode{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body), CreatedAt: nowMillis()}
	res, err := s.db.Exec(`INSERT INTO modes (title, body, created_at) VALUES (?, ?, ?)`, m.Title, m.Body, m.CreatedAt)
	// pathTaken tells any unique constraint apart; the only one here is the title.
	if pathTaken(err) {
		return nil, ErrModeTitleTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create mode: %w", err)
	}
	m.ID, _ = res.LastInsertId()
	return m, nil
}

// UpdateMode rewrites a mode. Every project linked to it gives the new text to
// its next conversation; conversations already under way keep their copy.
func (s *Store) UpdateMode(id int64, title, body string) error {
	res, err := s.db.Exec(`UPDATE modes SET title = ?, body = ? WHERE id = ?`,
		strings.TrimSpace(title), strings.TrimSpace(body), id)
	if pathTaken(err) {
		return ErrModeTitleTaken
	}
	if err != nil {
		return fmt.Errorf("update mode %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMode removes a mode and every link to it. Projects are left with no
// mode; tasks yet to start and schedules fall back to their project's.
// Conversations that already started keep the text they were given.
func (s *Store) DeleteMode(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete mode %d: %w", id, err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM modes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete mode %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	for _, q := range []string{
		`UPDATE projects SET mode_id = 0 WHERE mode_id = ?`,
		`UPDATE sessions SET mode_id = 0 WHERE mode_id = ?`,
		`UPDATE schedules SET mode_id = 0 WHERE mode_id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return fmt.Errorf("delete mode %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// SetProjectMode links a project to a mode, or to none with 0.
func (s *Store) SetProjectMode(id, modeID int64) error {
	res, err := s.db.Exec(`UPDATE projects SET mode_id = ? WHERE id = ?`, modeID, id)
	if err != nil {
		return fmt.Errorf("update project %d mode: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSessionMode picks the mode a task that has not started will open with.
func (s *Store) SetSessionMode(id string, modeID int64) error {
	res, err := s.db.Exec(`UPDATE sessions SET mode_id = ?, updated_at = ? WHERE id = ?`, modeID, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set session mode: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetScheduleMode picks the mode every run of a schedule opens with.
func (s *Store) SetScheduleMode(id, modeID int64) error {
	res, err := s.db.Exec(`UPDATE schedules SET mode_id = ? WHERE id = ?`, modeID, id)
	if err != nil {
		return fmt.Errorf("set schedule mode: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
