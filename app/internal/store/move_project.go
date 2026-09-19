package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var ErrProjectBusy = errors.New("stop the project's active or queued tasks before moving it")
var ErrMoveOrchestrator = errors.New("the orchestrator belongs to its profile and cannot be moved")

// MoveProjectTo transfers the complete relational graph in one attached-database
// transaction. Integer IDs are shifted past the destination's IDs; session UUIDs
// stay stable. Callers must exclude runner starts while this executes.
func (s *Store) MoveProjectTo(dst *Store, id int64) (int64, error) {
	if s.path == dst.path {
		return 0, errors.New("choose another profile")
	}
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `ATTACH DATABASE ? AS destination`, dst.path); err != nil {
		return 0, err
	}
	defer conn.ExecContext(ctx, `DETACH DATABASE destination`)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Take both write locks before reading IDs or checking work.
	for _, schema := range []string{"main", "destination"} {
		if _, err = tx.Exec(`UPDATE ` + schema + `.projects SET id = id WHERE 0`); err != nil {
			return 0, err
		}
	}
	var kind string
	if err = tx.QueryRow(`SELECT kind FROM main.projects WHERE id = ?`, id).Scan(&kind); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, err
	}
	if kind == "orchestrator" {
		return 0, ErrMoveOrchestrator
	}
	var busy bool
	if err = tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM main.sessions s WHERE s.project_id = ? AND
 (s.status = 'running' OR EXISTS (SELECT 1 FROM main.queued_messages q WHERE q.session_id = s.id)))
 OR EXISTS (SELECT 1 FROM main.schedule_runs r JOIN main.schedules j ON j.id = r.schedule_id WHERE j.project_id = ? AND r.status = 'running')`, id, id).Scan(&busy); err != nil {
		return 0, err
	}
	if busy {
		return 0, ErrProjectBusy
	}

	tables := []string{"projects", "schedules", "sessions", "turns", "messages", "schedule_runs"}
	offsets := map[string]int64{}
	for _, table := range tables {
		if table == "sessions" {
			continue
		}
		var max int64
		if err = tx.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM destination.` + table).Scan(&max); err != nil {
			return 0, err
		}
		offsets[table] = max
	}
	projectID := id + offsets["projects"]
	sessions := `SELECT id FROM main.sessions WHERE project_id = ?`
	filters := map[string]string{
		"projects": "id = ?", "schedules": "project_id = ?", "sessions": "project_id = ?",
		"turns": "session_id IN (" + sessions + ")", "messages": "session_id IN (" + sessions + ")",
		"schedule_runs": "schedule_id IN (SELECT id FROM main.schedules WHERE project_id = ?)",
	}
	for _, table := range tables {
		// Read column names so usage/history additions are preserved automatically.
		rows, err := tx.Query(`SELECT * FROM main.` + table + ` LIMIT 0`)
		if err != nil {
			return 0, err
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return 0, err
		}
		names, values := make([]string, len(columns)), make([]string, len(columns))
		for i, column := range columns {
			name := `"` + column + `"`
			names[i], values[i] = name, name
			switch column {
			case "id":
				if table != "sessions" {
					values[i] = fmt.Sprintf(`id + %d`, offsets[table])
				}
			case "project_id":
				values[i] = fmt.Sprint(projectID)
			case "schedule_id":
				values[i] = fmt.Sprintf(`CASE WHEN schedule_id = 0 THEN 0 ELSE schedule_id + %d END`, offsets["schedules"])
			case "turn_id":
				values[i] = fmt.Sprintf(`CASE WHEN turn_id IS NULL OR turn_id = 0 THEN turn_id ELSE turn_id + %d END`, offsets["turns"])
			case "account_id":
				values[i] = "0"
			case "provider_session_id":
				values[i] = `CASE WHEN account_id = 0 THEN provider_session_id ELSE '' END`
			case "paused":
				values[i] = "1"
			case "position":
				if table == "projects" {
					values[i] = `(SELECT CASE (SELECT new_item_position FROM destination.general_config WHERE id = 1) WHEN 'bottom' THEN COALESCE(MAX(position), 0) + 1 ELSE COALESCE(MIN(position), 0) - 1 END FROM destination.projects)`
				}
			}
		}
		query := `INSERT INTO destination.` + table + ` (` + strings.Join(names, ",") + `) SELECT ` + strings.Join(values, ",") + ` FROM main.` + table + ` WHERE ` + filters[table]
		if _, err = tx.Exec(query, id); err != nil {
			if table == "projects" && pathTaken(err) {
				return 0, ErrPathInUse
			}
			return 0, fmt.Errorf("move %s: %w", table, err)
		}
	}
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	if err = copyProjectFiles(tx, s.dir, dst.dir, id, projectID, &created); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`DELETE FROM main.projects WHERE id = ?`, id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return projectID, nil
}

var movedAttachment = regexp.MustCompile(`/api/attachments/([a-f0-9]{64}\.(?:png|jpg|gif|webp))`)

func copyProjectFiles(tx *sql.Tx, source, destination string, id, projectID int64, created *[]string) error {
	paths := map[string]string{}
	rows, err := tx.Query(`SELECT content FROM main.messages WHERE session_id IN (SELECT id FROM main.sessions WHERE project_id = ?)
 UNION ALL SELECT prompt FROM main.schedules WHERE project_id = ?
 UNION ALL SELECT prompt FROM main.projects WHERE id = ?
 UNION ALL SELECT project_prompt FROM main.sessions WHERE project_id = ?`, id, id, id, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var content string
		if err = rows.Scan(&content); err != nil {
			rows.Close()
			return err
		}
		for _, match := range movedAttachment.FindAllStringSubmatch(content, -1) {
			relative := filepath.Join("attachments", match[1])
			paths[relative] = relative
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT provider, provider_session_id FROM main.sessions WHERE project_id = ? AND account_id = 0 AND provider_session_id LIKE 'direct-%'`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var provider, thread string
		if err = rows.Scan(&provider, &thread); err != nil {
			rows.Close()
			return err
		}
		if _, err = uuid.Parse(strings.TrimPrefix(thread, "direct-")); err != nil || provider == "." || provider == ".." || strings.ContainsAny(provider, `/\`) {
			rows.Close()
			return errors.New("invalid API conversation path")
		}
		relative := filepath.Join("api-providers", provider, "sessions", thread+".json")
		if _, exists := paths[relative]; !exists {
			paths[relative] = filepath.Join("api-providers", provider, "sessions", "direct-"+uuid.NewString()+".json")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for relative, target := range paths {
		if relative != target {
			oldThread := strings.TrimSuffix(filepath.Base(relative), ".json")
			newThread := strings.TrimSuffix(filepath.Base(target), ".json")
			if _, err = tx.Exec(`UPDATE destination.sessions SET provider_session_id = ? WHERE project_id = ? AND provider_session_id = ?`, newThread, projectID, oldThread); err != nil {
				return err
			}
		}
		if err = copyMoveFile(filepath.Join(source, relative), filepath.Join(destination, target), created); err != nil {
			return fmt.Errorf("copy project file: %w", err)
		}
	}
	return nil
}

func copyMoveFile(source, destination string, created *[]string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		// Attachments are content-addressed and may already belong to another project.
		if filepath.Base(filepath.Dir(destination)) == "attachments" {
			return nil
		}
		return errors.New("the destination already contains this API conversation")
	}
	if err != nil {
		return err
	}
	*created = append(*created, destination)
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
