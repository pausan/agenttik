package store

import (
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

// recentSessionsPerProject is how many session titles the Projects sidebar shows.
const recentSessionsPerProject = 5

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
	p.RecentSessions = []SessionRef{} // the field is always an array, never null
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt)
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

// ListProjects returns every project with the titles of its most recent
// sessions attached.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, path, created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for i := range projects {
		refs, err := s.recentSessions(projects[i].ID, recentSessionsPerProject)
		if err != nil {
			return nil, err
		}
		projects[i].RecentSessions = refs
	}
	return projects, nil
}

func (s *Store) recentSessions(projectID int64, limit int) ([]SessionRef, error) {
	rows, err := s.db.Query(
		`SELECT id, title, status FROM sessions
		 WHERE project_id = ? ORDER BY last_active_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent sessions for project %d: %w", projectID, err)
	}
	defer rows.Close()

	refs := []SessionRef{}
	for rows.Next() {
		var r SessionRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Status); err != nil {
			return nil, fmt.Errorf("recent sessions for project %d: %w", projectID, err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}
