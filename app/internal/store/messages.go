package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) AddMessage(sessionID string, turnID int64, role, content string) (*Message, error) {
	m := &Message{SessionID: sessionID, TurnID: turnID, Role: role,
		Content: content, CreatedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO messages (session_id, turn_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.SessionID, m.TurnID, m.Role, m.Content, m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}
	m.ID, _ = res.LastInsertId()
	return m, nil
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
