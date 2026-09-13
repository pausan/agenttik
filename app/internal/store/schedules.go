package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Running is a subquery rather than a column: a run row is already written per
// fire, so asking whether one is still open costs nothing extra and cannot
// drift out of step with a column somebody forgot to clear.
const scheduleCols = `s.id, s.project_id, s.title, s.prompt, s.provider, s.account_id, s.model, s.effort,
	s.permission, s.every, s.interval_minutes, s.at_minute, s.anchor_at, s.remaining,
	s.paused, s.next_run_at, s.created_at, s.done_at, s.position,
	EXISTS(SELECT 1 FROM schedule_runs r WHERE r.schedule_id = s.id AND r.status = '` + RunRunning + `'),
	p.name, p.path`

func scanSchedule(sc interface{ Scan(...any) error }) (*Schedule, error) {
	var v Schedule
	err := sc.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Prompt, &v.Provider, &v.AccountID, &v.Model, &v.Effort,
		&v.Permission, &v.Every, &v.IntervalMinutes, &v.AtMinute, &v.AnchorAt, &v.Remaining,
		&v.Paused, &v.NextRunAt, &v.CreatedAt, &v.DoneAt, &v.Position,
		&v.Running, &v.ProjectName, &v.ProjectPath)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

const scheduleFrom = ` FROM schedules s JOIN projects p ON p.id = s.project_id`

func (s *Store) CreateSchedule(v *Schedule) error {
	v.CreatedAt = nowMillis()
	if v.AnchorAt == 0 {
		v.AnchorAt = v.CreatedAt
	}
	res, err := s.db.Exec(
		`INSERT INTO schedules (project_id, title, prompt, provider, account_id, model, effort, permission,
		    every, interval_minutes, at_minute, anchor_at, remaining, paused, next_run_at,
		    created_at, position)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,
		    (SELECT COALESCE(MAX(position), 0) + 1 FROM schedules WHERE project_id = ?))`,
		v.ProjectID, v.Title, v.Prompt, v.Provider, v.AccountID, v.Model, v.Effort, v.Permission,
		v.Every, v.IntervalMinutes, v.AtMinute, v.AnchorAt, v.Remaining, v.Paused, v.NextRunAt,
		v.CreatedAt, v.ProjectID)
	if err != nil {
		return fmt.Errorf("create schedule: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) GetSchedule(id int64) (*Schedule, error) {
	row := s.db.QueryRow(`SELECT `+scheduleCols+scheduleFrom+` WHERE s.id = ?`, id)
	v, err := scanSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule %d: %w", id, err)
	}
	return v, nil
}

// ScheduleFilter drives the sidebar lists. It mirrors SessionFilter, since the
// two are shown one above the other and answer the same questions.
type ScheduleFilter struct {
	Since       int64 // unix millis; 0 means no lower bound
	ProjectID   int64 // 0 means all projects
	Query       string
	Limit       int
	ExcludeDone bool
}

// ListSchedules orders one project's schedules by the order the sidebar was
// dragged into, and every other listing by the run coming up soonest. A
// schedule with nothing due — paused, or out of runs — sorts last rather than
// first, which is what a next_run_at of 0 would otherwise do.
func (s *Store) ListSchedules(f ScheduleFilter) ([]Schedule, error) {
	var where []string
	var args []any
	if f.Since > 0 {
		where = append(where, "(s.created_at >= ? OR s.paused = 0 OR s.every = 'pinned')")
		args = append(args, f.Since)
	}
	if f.ProjectID > 0 {
		where = append(where, "s.project_id = ?")
		args = append(args, f.ProjectID)
	}
	if f.ExcludeDone {
		where = append(where, "s.done_at = 0")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(s.title LIKE ? OR s.prompt LIKE ? OR p.name LIKE ? OR p.path LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	query := `SELECT ` + scheduleCols + scheduleFrom
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	if f.ProjectID > 0 {
		query += ` ORDER BY (s.every = 'pinned') DESC, s.position, s.created_at`
	} else {
		query += ` ORDER BY (s.every = 'pinned') DESC, (s.next_run_at = 0), s.next_run_at, s.created_at DESC`
	}
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	out := []Schedule{}
	for rows.Next() {
		v, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("list schedules: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// DueSchedules is the one query the ticker runs. Paused, archived and
// exhausted schedules are excluded here rather than in the runner, so a tick
// that finds nothing touches one index and stops. An archived project's
// schedules go with it: a project nobody can see must not be starting tasks.
func (s *Store) DueSchedules(now int64) ([]Schedule, error) {
	rows, err := s.db.Query(`SELECT `+scheduleCols+scheduleFrom+
		` WHERE s.every <> 'pinned' AND s.paused = 0 AND s.done_at = 0 AND s.remaining <> 0 AND s.next_run_at <= ?
		  AND p.archived_at = 0
		  ORDER BY s.next_run_at`, now)
	if err != nil {
		return nil, fmt.Errorf("due schedules: %w", err)
	}
	defer rows.Close()

	out := []Schedule{}
	for rows.Next() {
		v, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("due schedules: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// SetScheduleNextRun stores when the next fire is due. The ticker writes it
// after every fire, so being due stays a single indexed comparison.
func (s *Store) SetScheduleNextRun(id, at int64) error {
	_, err := s.db.Exec(`UPDATE schedules SET next_run_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("set schedule next run: %w", err)
	}
	return nil
}

// SetScheduleRecurrence changes the clock a schedule runs on. The anchor comes
// in with it because the weekday and the day of the month are read off the
// moment the form was chosen: switching to weekly re-anchors, moving a weekly
// schedule's time does not.
func (s *Store) SetScheduleRecurrence(id int64, every string, intervalMinutes, atMinute, anchorAt int64) error {
	res, err := s.db.Exec(`UPDATE schedules SET every = ?, interval_minutes = ?, at_minute = ?,
		anchor_at = ? WHERE id = ?`, every, intervalMinutes, atMinute, anchorAt, id)
	if err != nil {
		return fmt.Errorf("set schedule recurrence: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSchedulePrompt changes the prompt every future run sends. Runs already
// spawned are ordinary sessions and keep the text they were started with.
func (s *Store) SetSchedulePrompt(id int64, prompt string) error {
	res, err := s.db.Exec(`UPDATE schedules SET prompt = ? WHERE id = ?`, prompt, id)
	if err != nil {
		return fmt.Errorf("set schedule prompt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetScheduleModel changes what future runs are sent to. The fields move
// together because they are one answer: an effort belongs to a model, a model
// to a provider, and the provider to the subscription that pays for it.
func (s *Store) SetScheduleModel(id int64, provider string, accountID int64, model, effort string) error {
	res, err := s.db.Exec(`UPDATE schedules SET provider = ?, account_id = ?, model = ?, effort = ? WHERE id = ?`,
		provider, accountID, model, effort, id)
	if err != nil {
		return fmt.Errorf("set schedule model: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RetitleSchedule replaces a title only if the row still holds the one the
// caller last wrote, and reports whether it did. The generated name arrives
// seconds after the placeholder it replaces, and by then the job may have been
// named by hand; that name wins. The same rule RetitleSession follows.
func (s *Store) RetitleSchedule(id int64, title, expect string) (bool, error) {
	res, err := s.db.Exec(`UPDATE schedules SET title = ? WHERE id = ? AND title = ?`,
		title, id, expect)
	if err != nil {
		return false, fmt.Errorf("retitle schedule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("retitle schedule: %w", err)
	}
	return n > 0, nil
}

func (s *Store) SetScheduleTitle(id int64, title string) error {
	res, err := s.db.Exec(`UPDATE schedules SET title = ? WHERE id = ?`, title, id)
	if err != nil {
		return fmt.Errorf("set schedule title: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetScheduleRemaining changes how many runs are left. Anything below -1 is
// -1: there is one way to say forever.
func (s *Store) SetScheduleRemaining(id, remaining int64) error {
	if remaining < -1 {
		remaining = -1
	}
	res, err := s.db.Exec(`UPDATE schedules SET remaining = ? WHERE id = ?`, remaining, id)
	if err != nil {
		return fmt.Errorf("set schedule remaining: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SpendScheduleRun takes one off the counter when a run finishes. Forever
// (-1) and finished (0) are both left alone, so the guard is in the statement
// rather than in a read the caller has to race against.
func (s *Store) SpendScheduleRun(id int64) error {
	_, err := s.db.Exec(`UPDATE schedules SET remaining = remaining - 1 WHERE id = ? AND remaining > 0`, id)
	if err != nil {
		return fmt.Errorf("spend schedule run: %w", err)
	}
	return nil
}

// SetSchedulePaused stops or restarts the clock. Resuming leaves next_run_at
// to the caller, which recomputes it from now: a pause holds runs back, it
// does not save them up.
func (s *Store) SetSchedulePaused(id int64, paused bool) error {
	res, err := s.db.Exec(`UPDATE schedules SET paused = ? WHERE id = ?`, paused, id)
	if err != nil {
		return fmt.Errorf("set schedule paused: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetScheduleDone archives a schedule, or restores it. Archiving also pauses:
// a hidden schedule that kept starting sessions is the one state nothing in
// the UI would explain, and restoring therefore hands back a paused one.
func (s *Store) SetScheduleDone(id int64, done bool) error {
	var at int64
	if done {
		at = nowMillis()
	}
	res, err := s.db.Exec(`UPDATE schedules SET done_at = ?, paused = CASE WHEN ? THEN 1 ELSE paused END
		WHERE id = ?`, at, done, id)
	if err != nil {
		return fmt.Errorf("set schedule done: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSchedule drops the schedule and its run history. The sessions it
// spawned are ordinary sessions and stay where they are.
func (s *Store) DeleteSchedule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderSchedules numbers the given schedules 1..n, as ReorderSessions does.
func (s *Store) ReorderSchedules(projectID int64, ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder schedules: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE schedules SET position = ? WHERE id = ? AND project_id = ?`)
	if err != nil {
		return fmt.Errorf("reorder schedules: %w", err)
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i+1, id, projectID); err != nil {
			return fmt.Errorf("reorder schedules: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reorder schedules: %w", err)
	}
	return nil
}

// StartScheduleRun records a fire that spawned a session.
func (s *Store) StartScheduleRun(scheduleID int64, sessionID string) (*ScheduleRun, error) {
	v := &ScheduleRun{ScheduleID: scheduleID, SessionID: sessionID,
		Status: RunRunning, StartedAt: nowMillis()}
	res, err := s.db.Exec(
		`INSERT INTO schedule_runs (schedule_id, session_id, status, started_at) VALUES (?,?,?,?)`,
		v.ScheduleID, v.SessionID, v.Status, v.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("start schedule run: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return v, nil
}

// SkipScheduleRun records a fire that was passed over because the schedule's
// previous run was still going. It is written so the gap in the list has a
// reason; it spends no run.
//
// A skip that follows another skip extends it rather than adding a row: the
// schedule's most recent run is checked, and if that is itself a skip its
// ended_at and count grow instead. A schedule stuck behind one slow run reads
// as one line — a count and a time range — not one row per fire it waited
// out. Anything else as the most recent row (a fresh schedule, or a run that
// has since finished) starts a new group of one.
func (s *Store) SkipScheduleRun(scheduleID int64) error {
	now := nowMillis()
	var lastID int64
	var lastStatus string
	err := s.db.QueryRow(
		`SELECT id, status FROM schedule_runs WHERE schedule_id = ? ORDER BY id DESC LIMIT 1`,
		scheduleID).Scan(&lastID, &lastStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("skip schedule run: %w", err)
	}
	if lastStatus == RunSkipped {
		_, err = s.db.Exec(
			`UPDATE schedule_runs SET ended_at = ?, count = count + 1 WHERE id = ?`, now, lastID)
	} else {
		_, err = s.db.Exec(
			`INSERT INTO schedule_runs (schedule_id, status, started_at, ended_at, count) VALUES (?,?,?,?,1)`,
			scheduleID, RunSkipped, now, now)
	}
	if err != nil {
		return fmt.Errorf("skip schedule run: %w", err)
	}
	return nil
}

// FinishScheduleRun closes the open run of one spawned session. Keyed by
// session rather than by run id, because that is what a finishing turn holds.
// It reports how many rows it closed, which is what tells a caller whether
// this turn really was the scheduled run or a later prompt in the same
// session: only the first spends a run.
func (s *Store) FinishScheduleRun(sessionID, status string) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE schedule_runs SET status = ?, ended_at = ? WHERE session_id = ? AND status = ?`,
		status, nowMillis(), sessionID, RunRunning)
	if err != nil {
		return 0, fmt.Errorf("finish schedule run: %w", err)
	}
	return res.RowsAffected()
}

// ListScheduleRuns is the schedule view's list, newest first. The session
// title comes along so a row needs no request of its own.
func (s *Store) ListScheduleRuns(scheduleID int64, limit int) ([]ScheduleRun, error) {
	query := `SELECT r.id, r.schedule_id, r.session_id, r.status, r.started_at, r.ended_at, r.count,
		COALESCE(sess.title, '')
		FROM schedule_runs r LEFT JOIN sessions sess ON sess.id = r.session_id
		WHERE r.schedule_id = ? ORDER BY r.id DESC`
	args := []any{scheduleID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedule runs: %w", err)
	}
	defer rows.Close()

	out := []ScheduleRun{}
	for rows.Next() {
		var v ScheduleRun
		if err := rows.Scan(&v.ID, &v.ScheduleID, &v.SessionID, &v.Status,
			&v.StartedAt, &v.EndedAt, &v.Count, &v.Title); err != nil {
			return nil, fmt.Errorf("list schedule runs: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResetRunningScheduleRuns clears the open runs a crash left behind. Without
// it a schedule whose process died mid-run would read as busy forever and skip
// every fire after it. An interrupted run spends nothing: it did not finish.
func (s *Store) ResetRunningScheduleRuns() error {
	_, err := s.db.Exec(`UPDATE schedule_runs SET status = ?, ended_at = ? WHERE status = ?`,
		RunInterrupted, nowMillis(), RunRunning)
	if err != nil {
		return fmt.Errorf("reset running schedule runs: %w", err)
	}
	return nil
}
