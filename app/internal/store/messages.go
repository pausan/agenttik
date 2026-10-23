package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// A turnID of 0 stores none: a prompt the agent has not started on belongs to
// no turn, as a queued prompt moved in by Stop does not.
func (s *Store) AddMessage(sessionID string, turnID int64, role, content string) (*Message, error) {
	m := &Message{SessionID: sessionID, TurnID: turnID, Role: role,
		Content: content, CreatedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO messages (session_id, turn_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.SessionID, turnOrNull(m.TurnID), m.Role, m.Content, m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}
	m.ID, _ = res.LastInsertId()
	return m, nil
}

func turnOrNull(turnID int64) any {
	if turnID == 0 {
		return nil
	}
	return turnID
}

// DeleteMessage drops one message, the undoing of an AddMessage whose purpose
// fell through.
func (s *Store) DeleteMessage(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete message %d: %w", id, err)
	}
	return nil
}

// MoveMessageToEnd gives a message without a turn to turnID, 0 for none, and
// puts it below everything else in its session. A prompt sent into a running
// turn is stored when it is accepted, which can be before the end of a reply
// it waits behind; it moves below that reply once the agent starts on it. Ids
// are the transcript's order, so a message with rows after it moves under a
// new id, and replaced is the id it had. One already last keeps its id, and
// replaced is 0. A message that has a turn is not found: of two callers after
// the same prompt, only one gets it.
func (s *Store) MoveMessageToEnd(id, turnID int64) (m *Message, replaced int64, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, fmt.Errorf("move message %d: %w", id, err)
	}
	defer tx.Rollback()
	var msg Message
	if err := tx.QueryRow(`SELECT id, session_id, role, content, created_at FROM messages
		WHERE id = ? AND turn_id IS NULL`, id).
		Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("move message %d: %w", id, err)
	}
	msg.TurnID = turnID
	var later bool
	if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM messages WHERE session_id = ? AND id > ?)`,
		msg.SessionID, id).Scan(&later); err != nil {
		return nil, 0, fmt.Errorf("move message %d: %w", id, err)
	}
	if !later {
		if _, err := tx.Exec(`UPDATE messages SET turn_id = ? WHERE id = ?`, turnOrNull(turnID), id); err != nil {
			return nil, 0, fmt.Errorf("move message %d: %w", id, err)
		}
	} else {
		res, err := tx.Exec(`INSERT INTO messages (session_id, turn_id, role, content, created_at)
			VALUES (?, ?, ?, ?, ?)`, msg.SessionID, turnOrNull(turnID), msg.Role, msg.Content, msg.CreatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("move message %d: %w", id, err)
		}
		if _, err := tx.Exec(`DELETE FROM messages WHERE id = ?`, id); err != nil {
			return nil, 0, fmt.Errorf("move message %d: %w", id, err)
		}
		msg.ID, _ = res.LastInsertId()
		replaced = id
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("move message %d: %w", id, err)
	}
	return &msg, replaced, nil
}

// MessageToQueue puts a prompt the agent never started on back in its
// session's queue, behind what waits there, with the provider and model it is
// to run on, and takes it out of the transcript, in one transaction. As with
// MoveMessageToEnd, a message that has a turn is not found.
func (s *Store) MessageToQueue(id int64, provider string, accountID int64, model, effort string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("requeue message %d: %w", id, err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO queued_messages (session_id, prompt, provider, account_id, model, effort, created_at)
		SELECT session_id, content, ?, ?, ?, ?, ? FROM messages WHERE id = ? AND turn_id IS NULL`,
		provider, accountID, model, effort, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("requeue message %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE id = ?`, id); err != nil {
		return fmt.Errorf("requeue message %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("requeue message %d: %w", id, err)
	}
	return nil
}

// SetMessageContent updates a task's state without changing its transcript position.
func (s *Store) SetMessageContent(id int64, content string) error {
	_, err := s.db.Exec(`UPDATE messages SET content = ? WHERE id = ?`, content, id)
	if err != nil {
		return fmt.Errorf("set message content %d: %w", id, err)
	}
	return nil
}

// GetMessage reads one message. The caller checks it belongs to the session it
// is acting on: an id alone says nothing about that.
func (s *Store) GetMessage(id int64) (*Message, error) {
	var m Message
	err := s.db.QueryRow(
		`SELECT id, session_id, COALESCE(turn_id,0), role, content, created_at
		 FROM messages WHERE id = ?`, id).Scan(&m.ID, &m.SessionID, &m.TurnID,
		&m.Role, &m.Content, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message %d: %w", id, err)
	}
	return &m, nil
}

// DeleteMessagesFrom drops a message and everything recorded after it, which
// is what rewriting a prompt does to the transcript below it. Ids are handed
// out in order, so ">= from" is "from here down".
//
// The turns those messages belonged to are left alone: they are what the run
// actually cost, and a transcript being rewritten does not unspend it.
func (s *Store) DeleteMessagesFrom(sessionID string, from int64) error {
	_, err := s.db.Exec(
		`DELETE FROM messages WHERE session_id = ? AND id >= ?`, sessionID, from)
	if err != nil {
		return fmt.Errorf("delete messages from %d: %w", from, err)
	}
	return nil
}

// LastReply is the last thing the agent said in a session, capped. It is what
// a finished task is summarised from: the closing prose of the final turn,
// which is where an agent says what it did. Prose is flushed before each tool
// call, so the newest assistant row is that closing text rather than the whole
// turn.
//
// The cap is generous next to a title's 600 characters and still bounded: a
// long reply is mostly the middle, and both the request and its answer are
// tokens spent on every archive.
func (s *Store) LastReply(sessionID string) (string, error) {
	var content string
	err := s.db.QueryRow(
		`SELECT substr(content, 1, 4000) FROM messages
		  WHERE session_id = ? AND role = ? ORDER BY id DESC LIMIT 1`,
		sessionID, RoleAssistant).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("last reply of %s: %w", sessionID, err)
	}
	return content, nil
}

func (s *Store) ListMessages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, COALESCE(turn_id,0), role, content, created_at
		 FROM messages WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.TurnID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ToolCalls lists a session's tool rows in order, for reading which files
// its calls named. Only the columns that needs are read.
func (s *Store) ToolCalls(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT COALESCE(turn_id,0), content FROM messages
		  WHERE session_id = ? AND role = ? ORDER BY id`, sessionID, RoleTool)
	if err != nil {
		return nil, fmt.Errorf("tool calls of %s: %w", sessionID, err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		m := Message{SessionID: sessionID, Role: RoleTool}
		if err := rows.Scan(&m.TurnID, &m.Content); err != nil {
			return nil, fmt.Errorf("tool calls of %s: %w", sessionID, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
