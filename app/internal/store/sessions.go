package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const sessionCols = `s.id, s.project_id, s.title, s.provider, s.provider_session_id,
	s.model, s.effort, s.permission, s.source, s.status,
	s.created_at, s.updated_at, s.last_active_at, s.done_at, s.position,
	(SELECT COUNT(*) FROM queued_messages q WHERE q.session_id = s.id), p.name, p.path`

func scanSession(sc interface{ Scan(...any) error }) (*Session, error) {
	var v Session
	err := sc.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Provider, &v.ProviderSessionID,
		&v.Model, &v.Effort, &v.Permission, &v.Source, &v.Status,
		&v.CreatedAt, &v.UpdatedAt, &v.LastActiveAt, &v.DoneAt, &v.Position,
		&v.QueueCount, &v.ProjectName, &v.ProjectPath)
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
		    model, effort, permission, source, status, created_at, updated_at, last_active_at, position)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,
		    (SELECT COALESCE(MAX(position), 0) + 1 FROM sessions WHERE project_id = ?))`,
		v.ID, v.ProjectID, v.Title, v.Provider, v.ProviderSessionID,
		v.Model, v.Effort, v.Permission, v.Source, v.Status,
		v.CreatedAt, v.UpdatedAt, v.LastActiveAt, v.ProjectID)
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

// SessionFilter drives the Sessions sidebar and the project view.
type SessionFilter struct {
	Since       int64 // unix millis; 0 means no lower bound
	ProjectID   int64 // 0 means all projects
	Query       string
	Limit       int
	ExcludeDone bool // the project views hide ticked-off sessions
}

// ListSessions orders one project's sessions by the order they were dragged
// into, and every other listing by running first, then recency.
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
	if f.ExcludeDone {
		where = append(where, "s.done_at = 0")
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
	if f.ProjectID > 0 {
		sqlStr += ` ORDER BY s.position, s.last_active_at DESC`
	} else {
		sqlStr += ` ORDER BY (s.status = '` + StatusRunning + `') DESC, s.last_active_at DESC`
	}
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

// SetSessionModel records a mid-session model choice. Changing provider also
// clears its opaque thread id: a Codex thread cannot be resumed by Claude,
// and vice versa.
func (s *Store) SetSessionModel(id, provider, model, effort string, resetProviderSession bool) error {
	query := `UPDATE sessions SET provider = ?, model = ?, effort = ?, updated_at = ? WHERE id = ?`
	args := []any{provider, model, effort, nowMillis(), id}
	if resetProviderSession {
		query = `UPDATE sessions SET provider = ?, provider_session_id = '', model = ?, effort = ?, updated_at = ? WHERE id = ?`
	}
	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("set session model: %w", err)
	}
	return nil
}

// SetSessionDone ticks a session off, or unticks it. Doing so is not activity,
// so last_active_at is left alone and the Sessions window keeps showing it.
func (s *Store) SetSessionDone(id string, done bool) error {
	var at int64
	if done {
		at = nowMillis()
	}
	res, err := s.db.Exec(`UPDATE sessions SET done_at = ?, updated_at = ? WHERE id = ?`,
		at, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set session done: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderSessions numbers the given sessions 1..n in the order supplied. Ids
// belonging to another project are ignored rather than moved.
func (s *Store) ReorderSessions(projectID int64, ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder sessions: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE sessions SET position = ? WHERE id = ? AND project_id = ?`)
	if err != nil {
		return fmt.Errorf("reorder sessions: %w", err)
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i+1, id, projectID); err != nil {
			return fmt.Errorf("reorder sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reorder sessions: %w", err)
	}
	return nil
}

// EnqueueMessage persists a prompt until the project runner selects its
// session. Queue order inside one session is always first in, first out.
func (s *Store) EnqueueMessage(sessionID, prompt string) (*QueuedMessage, error) {
	v := &QueuedMessage{SessionID: sessionID, Prompt: prompt, CreatedAt: nowMillis()}
	res, err := s.db.Exec(`INSERT INTO queued_messages (session_id, prompt, created_at) VALUES (?, ?, ?)`,
		v.SessionID, v.Prompt, v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("enqueue message: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return v, nil
}

// NextQueuedMessage returns the oldest prompt waiting in one session.
func (s *Store) NextQueuedMessage(sessionID string) (*QueuedMessage, error) {
	v := &QueuedMessage{}
	err := s.db.QueryRow(`SELECT id, session_id, prompt, created_at FROM queued_messages
		WHERE session_id = ? ORDER BY id LIMIT 1`, sessionID).
		Scan(&v.ID, &v.SessionID, &v.Prompt, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("next queued message: %w", err)
	}
	return v, nil
}

// RemoveQueuedMessage is called immediately after its turn has claimed it.
func (s *Store) RemoveQueuedMessage(id int64) error {
	_, err := s.db.Exec(`DELETE FROM queued_messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove queued message: %w", err)
	}
	return nil
}

func (s *Store) QueueCount(sessionID string) (int64, error) {
	var count int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("queue count: %w", err)
	}
	return count, nil
}

// NextQueuedSession follows the projects visible session order. A preferred
// session is returned first while it still has work, so a sessions own queue
// drains before the scheduler returns to the projects top-to-bottom scan.
func (s *Store) NextQueuedSession(projectID int64, preferredSessionID string) (*Session, error) {
	if preferredSessionID != "" {
		sess, err := s.GetSession(preferredSessionID)
		if err != nil {
			return nil, err
		}
		if sess.ProjectID == projectID && sess.QueueCount > 0 {
			return sess, nil
		}
	}
	rows, err := s.ListSessions(SessionFilter{ProjectID: projectID, ExcludeDone: true})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].QueueCount > 0 {
			return &rows[i], nil
		}
	}
	return nil, nil
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
