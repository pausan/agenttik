package store

import (
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func (s *Store) CreateProject(name, path string) (*Project, error) {
	p := &Project{Name: name, Path: path, CreatedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO projects (name, path, created_at) VALUES (?, ?, ?)`,
		p.Name, p.Path, p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return p, nil
}

func (s *Store) GetProject(id int64) (*Project, error) {
	var p Project
	// Both lists are always arrays, never null, so the UI iterates without a guard.
	p.RecentSessions, p.Schedules = []SessionRef{}, []Schedule{}
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at, position FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project %d: %w", id, err)
	}
	return &p, nil
}

func (s *Store) SetProjectName(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("rename project %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProject(id int64) error {
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListProjects returns every project with its open session titles attached.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, path, created_at, position FROM projects ORDER BY position, name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for i := range projects {
		refs, err := s.recentSessions(projects[i].ID, 0)
		if err != nil {
			return nil, err
		}
		projects[i].RecentSessions = refs
		// The sidebar draws schedules above sessions, so they travel with the
		// project rather than costing a request per project.
		schedules, err := s.ListSchedules(ScheduleFilter{ProjectID: projects[i].ID, ExcludeDone: true})
		if err != nil {
			return nil, err
		}
		projects[i].Schedules = schedules
	}
	return projects, nil
}

// recentSessions feeds the Projects sidebar, so it hides ticked-off sessions
// and follows the order its sessions were dragged into. A limit of 0 means all.
func (s *Store) recentSessions(projectID int64, limit int) ([]SessionRef, error) {
	query := `SELECT id, title, status,
		(SELECT COUNT(*) FROM queued_messages q WHERE q.session_id = sessions.id) FROM sessions
		 WHERE project_id = ? AND done_at = 0
		 ORDER BY position, last_active_at DESC`
	args := []any{projectID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("recent sessions for project %d: %w", projectID, err)
	}
	defer rows.Close()

	refs := []SessionRef{}
	for rows.Next() {
		var r SessionRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Status, &r.QueueCount); err != nil {
			return nil, fmt.Errorf("recent sessions for project %d: %w", projectID, err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// ReorderProjects numbers the supplied projects 1..n. The caller sends the
// whole visible list, so a freshly created project can retain its 0 position
// until the user explicitly places it.
func (s *Store) ReorderProjects(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder projects: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE projects SET position = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("reorder projects: %w", err)
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i+1, id); err != nil {
			return fmt.Errorf("reorder projects: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reorder projects: %w", err)
	}
	return nil
}
