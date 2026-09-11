package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// firstPromptCol is the prompt a session opened with, which the sidebar shows
// on hover. It is capped in SQL: the tooltip clamps to five lines, and 600
// characters is already more than five of them hold at its width, so the rest
// would only weigh down every list. A session that has not run yet has no
// message of its own, so the prompt still waiting in its queue stands in.
const firstPromptCol = `COALESCE(
	(SELECT substr(m.content, 1, 600) FROM messages m
	  WHERE m.session_id = s.id AND m.role = '` + RoleUser + `' ORDER BY m.id LIMIT 1),
	(SELECT substr(q.prompt, 1, 600) FROM queued_messages q
	  WHERE q.session_id = s.id ORDER BY q.id LIMIT 1), '')`

const sessionCols = `s.id, s.project_id, s.title, s.provider, s.account_id, s.provider_session_id,
	s.model, s.effort, s.permission, s.source, s.status,
	s.created_at, s.updated_at, s.last_active_at, s.done_at, s.position,
	(SELECT COUNT(*) FROM queued_messages q WHERE q.session_id = s.id),
	s.schedule_id, s.project_prompt, p.name, p.path, ` + firstPromptCol

func scanSession(sc interface{ Scan(...any) error }) (*Session, error) {
	var v Session
	err := sc.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Provider, &v.AccountID, &v.ProviderSessionID,
		&v.Model, &v.Effort, &v.Permission, &v.Source, &v.Status,
		&v.CreatedAt, &v.UpdatedAt, &v.LastActiveAt, &v.DoneAt, &v.Position,
		&v.QueueCount, &v.ScheduleID, &v.ProjectPrompt, &v.ProjectName, &v.ProjectPath, &v.Prompt)
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
		`INSERT INTO sessions (id, project_id, title, provider, account_id, provider_session_id,
		    model, effort, permission, source, status, created_at, updated_at, last_active_at,
		    schedule_id, position)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,
		    (SELECT COALESCE(MAX(position), 0) + 1 FROM sessions WHERE project_id = ?))`,
		v.ID, v.ProjectID, v.Title, v.Provider, v.AccountID, v.ProviderSessionID,
		v.Model, v.Effort, v.Permission, v.Source, v.Status,
		v.CreatedAt, v.UpdatedAt, v.LastActiveAt, v.ScheduleID, v.ProjectID)
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
	OnlyDone    bool // ... and list them separately, under their own filter
}

// ListSessions orders one project's sessions by the order they were dragged
// into, and every other listing by running first, then recency. An
// archived-only listing is history rather than a list anyone arranged, so it
// comes back newest first whether or not it is scoped to a project.
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
	if f.OnlyDone {
		where = append(where, "s.done_at > 0")
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
	if f.OnlyDone {
		sqlStr += ` ORDER BY s.last_active_at DESC`
	} else if f.ProjectID > 0 {
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

// RetitleSession replaces a title only if the row still holds the one the
// caller last wrote, and reports whether it did. A generated title arrives
// seconds after the placeholder it replaces, and by then the user may have
// named the task themselves; their name wins.
func (s *Store) RetitleSession(id, title, expect string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ? AND title = ?`,
		title, nowMillis(), id, expect)
	if err != nil {
		return false, fmt.Errorf("retitle session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("retitle session: %w", err)
	}
	return n > 0, nil
}

// SetSessionModel records a mid-session model choice. Changing provider also
// clears its opaque thread id: a Codex thread cannot be resumed by Claude,
// and vice versa.
// SetSessionProjectPrompt records the project prompt a conversation was opened
// with, which is prepended to the first prompt its provider is given. Written
// once, when the conversation accepts its first prompt: from then on it is
// what this conversation was told, not what the project currently says.
func (s *Store) SetSessionProjectPrompt(id, prompt string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET project_prompt = ? WHERE id = ?`, prompt, id)
	if err != nil {
		return fmt.Errorf("set project prompt on session %s: %w", id, err)
	}
	return nil
}

func (s *Store) SetSessionModel(id, provider string, accountID int64, model, effort string, resetProviderSession bool) error {
	query := `UPDATE sessions SET provider = ?, account_id = ?, model = ?, effort = ?, updated_at = ? WHERE id = ?`
	args := []any{provider, accountID, model, effort, nowMillis(), id}
	if resetProviderSession {
		query = `UPDATE sessions SET provider = ?, account_id = ?, provider_session_id = '', model = ?, effort = ?, updated_at = ? WHERE id = ?`
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

// queuedCols is one column list for every read of the queue, so a row scans
// the same way wherever it is read from.
const queuedCols = `id, session_id, prompt, provider, account_id, model, effort, created_at,
	retry_at, retry_count, retry_error`

// queuedOrder is the order a session's queue runs in: when each prompt was
// accepted, id breaking a tie inside one millisecond. Acceptance time rather
// than id, because a prompt put back after an outage keeps the time it was
// accepted with and so returns to the place it left. See RequeueMessage.
const queuedOrder = ` ORDER BY created_at, id`

func scanQueued(row interface{ Scan(...any) error }) (*QueuedMessage, error) {
	v := &QueuedMessage{}
	err := row.Scan(&v.ID, &v.SessionID, &v.Prompt, &v.Provider, &v.AccountID, &v.Model, &v.Effort,
		&v.CreatedAt, &v.RetryAt, &v.RetryCount, &v.RetryError)
	return v, err
}

// EnqueueMessage persists a prompt and its chosen provider settings until the
// project runner selects it. Queue order inside one session is first in, first
// out.
func (s *Store) EnqueueMessage(sessionID, prompt, provider string, accountID int64, model, effort string) (*QueuedMessage, error) {
	v := &QueuedMessage{SessionID: sessionID, Prompt: prompt, Provider: provider, AccountID: accountID, Model: model, Effort: effort, CreatedAt: nowMillis()}
	res, err := s.db.Exec(`INSERT INTO queued_messages (session_id, prompt, provider, account_id, model, effort, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, v.SessionID, v.Prompt, v.Provider, v.AccountID, v.Model, v.Effort, v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("enqueue message: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return v, nil
}

// RequeueMessage puts a prompt back after its turn failed on a provider that
// was away, held until retryAt and carrying the failure that put it there.
//
// It keeps the acceptance time the prompt already had, which is what returns
// it to the place in the queue it left rather than behind whatever was queued
// while it ran. The waiting timer in the transcript keeps counting from the
// same moment for the same reason: the wait the user is being told about
// started when they sent it. See 045-provider-outage-retry.md.
func (s *Store) RequeueMessage(v QueuedMessage, retryAt int64, failure string) (*QueuedMessage, error) {
	v.RetryAt, v.RetryCount, v.RetryError = retryAt, v.RetryCount+1, failure
	res, err := s.db.Exec(`INSERT INTO queued_messages
		(session_id, prompt, provider, account_id, model, effort, created_at, retry_at, retry_count, retry_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.SessionID, v.Prompt, v.Provider, v.AccountID, v.Model, v.Effort, v.CreatedAt,
		v.RetryAt, v.RetryCount, v.RetryError)
	if err != nil {
		return nil, fmt.Errorf("requeue message: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return &v, nil
}

// NextQueuedMessage returns the oldest prompt in one session that is ready to
// run at now. A prompt held back for a retry is still queued — it is drawn,
// counted and can be forced — but it is not offered to the scheduler until
// its wait is over.
func (s *Store) NextQueuedMessage(sessionID string, now int64) (*QueuedMessage, error) {
	v, err := scanQueued(s.db.QueryRow(`SELECT `+queuedCols+` FROM queued_messages
		WHERE session_id = ? AND retry_at <= ?`+queuedOrder+` LIMIT 1`, sessionID, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("next queued message: %w", err)
	}
	return v, nil
}

// GetQueuedMessage returns one waiting prompt. Its session id lets callers
// verify that a queue item belongs to the session they are acting on.
func (s *Store) GetQueuedMessage(id int64) (*QueuedMessage, error) {
	v, err := scanQueued(s.db.QueryRow(`SELECT `+queuedCols+` FROM queued_messages WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get queued message %d: %w", id, err)
	}
	return v, nil
}

// ListQueuedMessages returns everything still waiting in one session, oldest
// first, which is the order the scheduler will run them in. Always a slice, so
// the transcript can iterate it without a guard.
func (s *Store) ListQueuedMessages(sessionID string) ([]QueuedMessage, error) {
	rows, err := s.db.Query(`SELECT `+queuedCols+` FROM queued_messages
		WHERE session_id = ?`+queuedOrder, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list queued messages: %w", err)
	}
	defer rows.Close()

	out := []QueuedMessage{}
	for rows.Next() {
		v, err := scanQueued(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued messages: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// RetryProjects names the projects holding a prompt whose wait is over, so the
// clock that retries them scans one indexed table instead of every session.
// A prompt only waiting its turn has retry_at 0 and is not one of these: the
// scheduler already reaches it when the project falls idle.
func (s *Store) RetryProjects(now int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT DISTINCT s.project_id
		FROM queued_messages q JOIN sessions s ON s.id = q.session_id
		WHERE q.retry_at > 0 AND q.retry_at <= ?`, now)
	if err != nil {
		return nil, fmt.Errorf("retry projects: %w", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("retry projects: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetQueuedMessage changes one waiting prompt's text and model choice without
// altering the session default. The runner applies the choice when it claims
// the prompt.
func (s *Store) SetQueuedMessage(id int64, prompt, provider string, accountID int64, model, effort string) error {
	res, err := s.db.Exec(`UPDATE queued_messages SET prompt = ?, provider = ?, account_id = ?, model = ?, effort = ? WHERE id = ?`,
		prompt, provider, accountID, model, effort, id)
	if err != nil {
		return fmt.Errorf("set queued message: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set queued message: %w", err)
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveQueuedMessage is called immediately after its turn has claimed it.
func (s *Store) RemoveQueuedMessage(id int64) error {
	_, err := s.db.Exec(`DELETE FROM queued_messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove queued message: %w", err)
	}
	return nil
}

// RemoveQueuedMessages drops every prompt waiting for one session. Stopping a
// queued session uses this so the scheduler cannot pick it up later.
func (s *Store) RemoveQueuedMessages(sessionID string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM queued_messages WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, fmt.Errorf("remove queued messages: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) QueueCount(sessionID string) (int64, error) {
	var count int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("queue count: %w", err)
	}
	return count, nil
}

// NextQueuedRun picks the session whose queue runs next in a project, and the
// prompt it will run. It follows the projects visible session order, and a
// preferred session is returned first while it still has work, so a sessions
// own queue drains before the scheduler returns to the projects top-to-bottom
// scan.
//
// Session and prompt are chosen together because a session can hold prompts
// and still have nothing to run: one waiting out a provider outage is passed
// over so the ready work below it is not starved for the length of the wait.
func (s *Store) NextQueuedRun(projectID int64, preferredSessionID string, now int64) (*Session, *QueuedMessage, error) {
	ready := func(sess *Session) (*Session, *QueuedMessage, error) {
		if sess.QueueCount == 0 {
			return nil, nil, nil
		}
		queued, err := s.NextQueuedMessage(sess.ID, now)
		if err != nil || queued == nil {
			return nil, nil, err
		}
		return sess, queued, nil
	}

	if preferredSessionID != "" {
		sess, err := s.GetSession(preferredSessionID)
		if err != nil {
			return nil, nil, err
		}
		if sess.ProjectID == projectID {
			if sess, queued, err := ready(sess); err != nil || queued != nil {
				return sess, queued, err
			}
		}
	}
	rows, err := s.ListSessions(SessionFilter{ProjectID: projectID, ExcludeDone: true})
	if err != nil {
		return nil, nil, err
	}
	for i := range rows {
		if rows[i].ID == preferredSessionID {
			continue
		}
		if sess, queued, err := ready(&rows[i]); err != nil || queued != nil {
			return sess, queued, err
		}
	}
	return nil, nil, nil
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
