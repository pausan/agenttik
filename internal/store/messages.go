package store

import "fmt"

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
