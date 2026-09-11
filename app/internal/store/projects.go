package store

import (
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrNotFound = errors.New("not found")

// ErrPathInUse is returned when a folder is already another project's. The
// path column is unique: two projects on one folder would give its Tree, its
// Changed pane and its turns two owners.
var ErrPathInUse = errors.New("another project already uses that folder")

// pathTaken tells the unique-path constraint apart from a real database
// failure, so the one the user can act on does not reach them as SQLite's
// own wording.
func pathTaken(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// CreateProject adds a project below the ones already there: it takes the
// next position rather than the default 0, which would put it at the top of a
// list the user has already arranged. Archived projects count, so restoring
// one does not land it on top of a newer project's number.
func (s *Store) CreateProject(name, path string) (*Project, error) {
	p := &Project{Name: name, Path: path, CreatedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO projects (name, path, created_at, position)
		 VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM projects))`,
		p.Name, p.Path, p.CreatedAt)
	if pathTaken(err) {
		return nil, ErrPathInUse
	}
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
		`SELECT id, name, path, created_at, position, archived_at, prompt FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.ArchivedAt, &p.Prompt)
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

// SetProjectPath repoints a project at a new folder, for when the one on disk
// has moved. Nothing under the project is touched; only where future turns,
// the Tree and the Changed pane look for it changes.
func (s *Store) SetProjectPath(id int64, path string) error {
	res, err := s.db.Exec(`UPDATE projects SET path = ? WHERE id = ?`, path, id)
	if pathTaken(err) {
		return ErrPathInUse
	}
	if err != nil {
		return fmt.Errorf("update project %d path: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectPrompt sets the standing prompt every conversation started in the
// project is given before its first prompt. Conversations already under way
// keep the copy they were opened with, so this only reaches the next one.
// Empty turns the injection off. See 047-project-prompt.md.
func (s *Store) SetProjectPrompt(id int64, prompt string) error {
	res, err := s.db.Exec(`UPDATE projects SET prompt = ? WHERE id = ?`, prompt, id)
	if err != nil {
		return fmt.Errorf("update project %d prompt: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectArchived puts a project away, or brings it back. Nothing under it
// is touched: its tasks, its history and its position all stay, so restoring
// puts it back where it was. Deleting is the destructive one.
func (s *Store) SetProjectArchived(id int64, archived bool) error {
	var at int64
	if archived {
		at = nowMillis()
	}
	res, err := s.db.Exec(`UPDATE projects SET archived_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("archive project %d: %w", id, err)
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

// ListProjects returns every active project with its open session titles
// attached. Archived ones are ArchivedProjects' business.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(
		`SELECT id, name, path, created_at, position, prompt FROM projects
		 WHERE archived_at = 0 ORDER BY position, name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.Prompt); err != nil {
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

// ArchivedProjects lists what has been put away, most recently archived
// first. Settings is the only place that shows them, and it shows a name, a
// folder and a date, so neither the session titles nor the schedules
// ListProjects attaches are read.
func (s *Store) ArchivedProjects() ([]Project, error) {
	rows, err := s.db.Query(
		`SELECT id, name, path, created_at, position, archived_at FROM projects
		 WHERE archived_at <> 0 ORDER BY archived_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list archived projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		p := Project{RecentSessions: []SessionRef{}, Schedules: []Schedule{}}
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.ArchivedAt); err != nil {
			return nil, fmt.Errorf("list archived projects: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// recentSessions feeds the Projects sidebar, so it hides ticked-off sessions
// and follows the order its sessions were dragged into. A limit of 0 means all.
func (s *Store) recentSessions(projectID int64, limit int) ([]SessionRef, error) {
	query := `SELECT s.id, s.title, s.status,
		(SELECT COUNT(*) FROM queued_messages q WHERE q.session_id = s.id),
		s.schedule_id, ` +
		firstPromptCol + ` FROM sessions s
		 WHERE s.project_id = ? AND s.done_at = 0
		 ORDER BY s.position, s.last_active_at DESC`
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
		if err := rows.Scan(&r.ID, &r.Title, &r.Status, &r.QueueCount,
			&r.ScheduleID, &r.Prompt); err != nil {
			return nil, fmt.Errorf("recent sessions for project %d: %w", projectID, err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// ReorderProjects numbers the supplied projects 1..n. The caller sends the
// whole visible list, so the numbers it writes are dense and a project added
// afterwards still sorts below them all.
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
