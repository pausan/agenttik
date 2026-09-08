package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const sessionCols = `s.id, s.project_id, s.title, s.provider, s.provider_session_id,
	s.model, s.effort, s.permission, s.source, s.status,
	s.created_at, s.updated_at, s.last_active_at, p.name, p.path`

func scanSession(sc interface{ Scan(...any) error }) (*Session, error) {
	var v Session
	err := sc.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Provider, &v.ProviderSessionID,
		&v.Model, &v.Effort, &v.Permission, &v.Source, &v.Status,
		&v.CreatedAt, &v.UpdatedAt, &v.LastActiveAt, &v.ProjectName, &v.ProjectPath)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) CreateSession(v *Session) error {
	now := nowMillis()
	v.CreatedAt, v.UpdatedAt, v.LastActiveAt = now, now, now
	if v.Source == "" {
		v.Source = "agenttik"
	}
	if v.Status == "" {
		v.Status = StatusIdle
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, project_id, title, provider, provider_session_id,
		    model, effort, permission, source, status, created_at, updated_at, last_active_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.ProjectID, v.Title, v.Provider, v.ProviderSessionID,
		v.Model, v.Effort, v.Permission, v.Source, v.Status,
		v.CreatedAt, v.UpdatedAt, v.LastActiveAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT `+sessionCols+` FROM sessions s JOIN projects p ON p.id = s.project_id
		 WHERE s.id = ?`, id)
	v, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return v, nil
}

// SessionFilter drives the Sessions sidebar.
type SessionFilter struct {
	Since     int64 // unix millis; 0 means no lower bound
	ProjectID int64 // 0 means all projects
	Query     string
	Limit     int
}

// ListSessions returns running sessions first, then the rest by recency.
func (s *Store) ListSessions(f SessionFilter) ([]Session, error) {
	var where []string
	var args []any
	// Running sessions always show, however old their last activity is.
	if f.Since > 0 {
		where = append(where, "(s.last_active_at >= ? OR s.status = ?)")
		args = append(args, f.Since, StatusRunning)
	}
	if f.ProjectID > 0 {
		where = append(where, "s.project_id = ?")
		args = append(args, f.ProjectID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(s.title LIKE ? OR p.name LIKE ? OR p.path LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	sqlStr := `SELECT ` + sessionCols + ` FROM sessions s JOIN projects p ON p.id = s.project_id`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += ` ORDER BY (s.status = '` + StatusRunning + `') DESC, s.last_active_at DESC`
	if f.Limit > 0 {
		sqlStr += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	out := []Session{}
	for rows.Next() {
		v, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list sessions: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (s *Store) SetSessionStatus(id, status string) error {
	now := nowMillis()
	_, err := s.db.Exec(
		`UPDATE sessions SET status = ?, updated_at = ?, last_active_at = ? WHERE id = ?`,
		status, now, now, id)
	if err != nil {
		return fmt.Errorf("set session status: %w", err)
	}
	return nil
}

func (s *Store) SetProviderSessionID(id, providerSessionID string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET provider_session_id = ?, updated_at = ? WHERE id = ?`,
		providerSessionID, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set provider session id: %w", err)
	}
	return nil
}

func (s *Store) SetSessionTitle(id, title string) error {
	_, err := s.db.Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set session title: %w", err)
	}
	return nil
}

// SetSessionModel records a mid-session switch of model or effort.
func (s *Store) SetSessionModel(id, model, effort string) error {
	_, err := s.db.Exec(`UPDATE sessions SET model = ?, effort = ?, updated_at = ? WHERE id = ?`,
		model, effort, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set session model: %w", err)
	}
	return nil
}

func (s *Store) DeleteSession(id string) error {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetRunningSessions clears the running flag left behind by a crash, since
// no turn survives a restart.
func (s *Store) ResetRunningSessions() error {
	_, err := s.db.Exec(`UPDATE sessions SET status = ? WHERE status = ?`, StatusIdle, StatusRunning)
	if err != nil {
		return fmt.Errorf("reset running sessions: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE turns SET status = 'interrupted', ended_at = ? WHERE status = 'running'`,
		nowMillis())
	if err != nil {
		return fmt.Errorf("reset running turns: %w", err)
	}
	return nil
}
