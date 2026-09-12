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

// CreateProject inserts at the configured end, counting archived positions too.
func (s *Store) CreateProject(name, path string) (*Project, error) {
	p := &Project{Name: name, Path: path, CreatedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO projects (name, path, created_at, position)
		 VALUES (?, ?, ?, (SELECT CASE (SELECT new_item_position FROM general_config WHERE id = 1)
		      WHEN 'bottom' THEN COALESCE(MAX(position), 0) + 1
		      ELSE COALESCE(MIN(position), 0) - 1 END FROM projects))`,
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
		`SELECT id, name, path, created_at, position, archived_at, hidden_at, hidden_position, prompt, kind
		 FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.ArchivedAt,
			&p.HiddenAt, &p.HiddenPosition, &p.Prompt, &p.Kind)
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
	query := `UPDATE projects SET archived_at = ?`
	if archived {
		// Archiving is the stronger put-away state. Clearing hidden state keeps
		// a later archive restore from silently returning to the hidden menu.
		query += `, hidden_at = 0, hidden_position = 0`
	}
	query += ` WHERE id = ?`
	res, err := s.db.Exec(query, at, id)
	if err != nil {
		return fmt.Errorf("archive project %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// orderedVisibleProjectIDs returns the order the Projects sidebar draws. It
// runs inside the caller's transaction so hiding/restoring can capture or
// reinsert a row without a concurrent reorder changing the index underneath.
func orderedVisibleProjectIDs(tx *sql.Tx) ([]int64, error) {
	rows, err := tx.Query(`SELECT id FROM projects
		WHERE archived_at = 0 AND hidden_at = 0
		ORDER BY (kind = 'orchestrator') DESC, position, name`)
	if err != nil {
		return nil, fmt.Errorf("list visible project order: %w", err)
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list visible project order: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list visible project order: %w", err)
	}
	return ids, nil
}

// SetProjectHidden hides a visible project or restores a hidden one. The
// hidden row keeps its visible index; restoring inserts it at that index and
// rewrites only the visible rows' dense positions. Nothing in the project is
// archived, stopped or deleted.
func (s *Store) SetProjectHidden(id int64, hidden bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("hide project: %w", err)
	}
	defer tx.Rollback()

	if hidden {
		ids, err := orderedVisibleProjectIDs(tx)
		if err != nil {
			return err
		}
		rank := 0
		for i, candidate := range ids {
			if candidate == id {
				rank = i + 1
				break
			}
		}
		if rank == 0 {
			var active bool
			if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM projects WHERE id = ? AND archived_at = 0)`, id).Scan(&active); err != nil {
				return fmt.Errorf("check project %d: %w", id, err)
			}
			if !active {
				return ErrNotFound
			}
			// The project is already hidden. Make the operation idempotent for
			// a second click or a duplicate request.
			return tx.Commit()
		}
		if _, err := tx.Exec(`UPDATE projects SET hidden_at = ?, hidden_position = ?
			WHERE id = ? AND archived_at = 0 AND hidden_at = 0`, nowMillis(), rank, id); err != nil {
			return fmt.Errorf("hide project %d: %w", id, err)
		}
	} else {
		var target int64
		var kind string
		err := tx.QueryRow(`SELECT hidden_position, kind FROM projects
			WHERE id = ? AND archived_at = 0 AND hidden_at <> 0`, id).Scan(&target, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			var active bool
			if checkErr := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM projects WHERE id = ? AND archived_at = 0)`, id).Scan(&active); checkErr != nil {
				return fmt.Errorf("check project %d: %w", id, checkErr)
			}
			if !active {
				return ErrNotFound
			}
			// The project is already visible. Make restore idempotent too.
			return tx.Commit()
		}
		if err != nil {
			return fmt.Errorf("read hidden project %d: %w", id, err)
		}

		ids, err := orderedVisibleProjectIDs(tx)
		if err != nil {
			return err
		}
		var orchestratorVisible bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM projects
			WHERE kind = 'orchestrator' AND archived_at = 0 AND hidden_at = 0)`).Scan(&orchestratorVisible); err != nil {
			return fmt.Errorf("check visible orchestrator: %w", err)
		}
		if kind == "orchestrator" {
			target = 1
		} else if orchestratorVisible && target < 2 {
			target = 2
		}
		if target < 1 {
			target = 1
		}
		if max := int64(len(ids) + 1); target > max {
			target = max
		}
		at := int(target - 1)
		ids = append(ids, 0)
		copy(ids[at+1:], ids[at:])
		ids[at] = id

		if _, err := tx.Exec(`UPDATE projects SET hidden_at = 0, hidden_position = 0 WHERE id = ?`, id); err != nil {
			return fmt.Errorf("restore project %d: %w", id, err)
		}
		stmt, err := tx.Prepare(`UPDATE projects SET position = ? WHERE id = ?`)
		if err != nil {
			return fmt.Errorf("restore project order: %w", err)
		}
		defer stmt.Close()
		for i, projectID := range ids {
			if _, err := stmt.Exec(i+1, projectID); err != nil {
				return fmt.Errorf("restore project order: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("hide project: %w", err)
	}
	return nil
}

func (s *Store) DeleteProject(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Only preferences survive an orchestrator deletion. The usual cascade
	// removes tasks and schedules; no project operation removes disk files.
	_, err = tx.Exec(`INSERT INTO orchestrator_config (id, name, path, prompt)
		SELECT 1, name, path, prompt FROM projects WHERE id = ? AND kind = 'orchestrator'
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, path = excluded.path, prompt = excluded.prompt`, id)
	if err != nil {
		return fmt.Errorf("save orchestrator options: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// ListProjects returns every visible active project with its open session
// titles attached. Archived and hidden ones have separate list methods.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(
		`SELECT id, name, path, created_at, position, prompt, kind FROM projects
		 WHERE archived_at = 0 AND hidden_at = 0
		 ORDER BY (kind = 'orchestrator') DESC, position, name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.Prompt, &p.Kind); err != nil {
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
		`SELECT id, name, path, created_at, position, archived_at, kind FROM projects
		 WHERE archived_at <> 0 ORDER BY archived_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list archived projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		p := Project{RecentSessions: []SessionRef{}, Schedules: []Schedule{}}
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.Position, &p.ArchivedAt, &p.Kind); err != nil {
			return nil, fmt.Errorf("list archived projects: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// HiddenProjects lists the small rows the sidebar's restore menu needs. It
// deliberately does not load paths, tasks or schedules: the menu only shows
// names and the ids used to restore them.
func (s *Store) HiddenProjects() ([]HiddenProject, error) {
	rows, err := s.db.Query(`SELECT id, name FROM projects
		WHERE archived_at = 0 AND hidden_at <> 0
		ORDER BY hidden_at DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list hidden projects: %w", err)
	}
	defer rows.Close()

	projects := []HiddenProject{}
	for rows.Next() {
		var p HiddenProject
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, fmt.Errorf("list hidden projects: %w", err)
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
// whole visible list, so the numbers it writes are dense.
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
