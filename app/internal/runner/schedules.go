// Schedules: a prompt plus a clock. See specs/028-scheduled-jobs.md.
package runner

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// EventScheduleChanged tells a window that a schedule fired, skipped, or spent
// a run. It names no schedule: the views re-read what they are showing, as the
// file watcher's event does.
const EventScheduleChanged agent.EventType = "schedule_changed"

// scheduleTick is the resolution of every schedule. A daily one set for 09:00
// fires within fifteen seconds of it — minute-accurate to the eye — and the
// idle cost of the whole feature is one indexed query every quarter minute.
const scheduleTick = 15 * time.Second

// defaultInterval is what an interval schedule falls back to, and the figure
// the dialog opens on.
const defaultInterval = 15 * time.Minute

// RunSchedules fires due schedules until ctx is cancelled. One goroutine for
// every schedule in the database, since being due is a single comparison.
func (r *Runner) RunSchedules(ctx context.Context) {
	ticker := time.NewTicker(scheduleTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.tickSchedules(now)
		}
	}
}

func (r *Runner) tickSchedules(now time.Time) {
	due, err := r.store.DueSchedules(now.UnixMilli())
	if err != nil {
		return
	}
	for i := range due {
		r.fireSchedule(&due[i], now)
	}
}

// fireSchedule spawns one run, or records a skip. Either way the next run is
// booked before it returns, so a schedule can never fire twice for one slot.
func (r *Runner) fireSchedule(s *store.Schedule, now time.Time) {
	prev := time.UnixMilli(s.NextRunAt)
	_ = r.store.SetScheduleNextRun(s.ID, AdvanceRun(s, prev, now).UnixMilli())
	defer r.publishSchedule(s.ProjectID)

	// Still busy with its own previous run. Queueing this one would only pile
	// four 15-minute runs onto 17:00 because 16:00 was slow, so it is passed
	// over — recorded, since a gap with no reason reads as a bug.
	if s.Running {
		_ = r.store.SkipScheduleRun(s.ID)
		return
	}

	sess := &store.Session{
		ID:         uuid.NewString(),
		ProjectID:  s.ProjectID,
		Title:      s.Title,
		Provider:   s.Provider,
		Model:      s.Model,
		Effort:     s.Effort,
		Permission: s.Permission,
		ScheduleID: s.ID,
	}
	if err := r.store.CreateSession(sess); err != nil {
		return
	}
	if _, err := r.store.StartScheduleRun(s.ID, sess.ID); err != nil {
		return
	}
	// Straight to Send: the project queue is for prompts waiting on a project,
	// and a schedule that waited would stop being a schedule.
	if _, err := r.Send(sess.ID, s.Prompt); err != nil {
		_, _ = r.store.FinishScheduleRun(sess.ID, store.RunError)
	}
}

// finishScheduledRun closes a spawned run when its turn ends. The counter goes
// down here rather than at spawn time, because what was asked for is a number
// of runs done, and a run that failed is still over.
//
// It is the run row that decides, not the session: a scheduled session the
// user later prompts by hand has no open run, so it spends nothing and is not
// archived a second time.
func (r *Runner) finishScheduledRun(sess *store.Session, turnStatus string) {
	status := store.RunDone
	if turnStatus != "ok" {
		status = store.RunError
	}
	closed, err := r.store.FinishScheduleRun(sess.ID, status)
	if err != nil || closed == 0 {
		return
	}
	_ = r.store.SpendScheduleRun(sess.ScheduleID)
	// The run leaves the project views and stays in the Sessions list, which
	// is what archiving already means. Otherwise forty runs bury the project
	// they belong to; the schedule's own view is where they are kept.
	_ = r.store.SetSessionDone(sess.ID, true)
	r.publishSchedule(sess.ProjectID)
}

func (r *Runner) publishSchedule(projectID int64) {
	r.hub.Publish(ProjectTopic(projectID), Event{ProjectID: projectID,
		Event: agent.Event{Type: EventScheduleChanged}})
}

// FirstRun is when a schedule should fire next, counting from now. Creating a
// schedule and resuming a paused one both use it: a pause holds runs back, it
// does not save them up.
func FirstRun(s *store.Schedule, now time.Time) time.Time {
	switch s.Every {
	case store.EveryDay:
		at := atClock(now, s.AtMinute)
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
		return at

	case store.EveryWeek:
		// The weekday is the one the schedule was created on. The dialog asks
		// for the three words the user gave it and nothing more; the anchor
		// answers the rest without another control.
		anchor := time.UnixMilli(s.AnchorAt).In(now.Location())
		at := atClock(now, s.AtMinute)
		at = at.AddDate(0, 0, (int(anchor.Weekday())-int(at.Weekday())+7)%7)
		if !at.After(now) {
			at = at.AddDate(0, 0, 7)
		}
		return at

	case store.EveryMonth:
		day := time.UnixMilli(s.AnchorAt).In(now.Location()).Day()
		at := monthDay(now.Year(), now.Month(), day, s.AtMinute, now.Location())
		if !at.After(now) {
			at = monthDay(now.Year(), now.Month()+1, day, s.AtMinute, now.Location())
		}
		return at

	default: // store.EveryInterval, and anything unrecognised
		return now.Add(intervalStep(s))
	}
}

// AdvanceRun books the run after one that has just fired. An interval counts
// from the slot rather than from now, so a cadence does not drift by however
// long the tick was late.
//
// Whole intervals are skipped in one step, not one tick at a time: a machine
// that was asleep for three hours catches up to the next slot instead of
// firing twelve times in a row. Nothing was in the way for those, so nothing
// is recorded as skipped either.
func AdvanceRun(s *store.Schedule, prev, now time.Time) time.Time {
	if s.Every != store.EveryInterval && s.Every != "" {
		return FirstRun(s, now)
	}
	step := intervalStep(s)
	next := prev.Add(step)
	if !next.After(now) {
		next = next.Add(step * (now.Sub(next)/step + 1))
	}
	return next
}

func intervalStep(s *store.Schedule) time.Duration {
	step := time.Duration(s.IntervalMinutes) * time.Minute
	if step < time.Minute {
		return defaultInterval
	}
	return step
}

// atClock is the given day at the schedule's hh:mm, in local time — the clock
// on the wall is what "every day at 9" is read off.
func atClock(day time.Time, atMinute int64) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(),
		int(atMinute/60), int(atMinute%60), 0, 0, day.Location())
}

// monthDay clamps to the month's last day, so a schedule anchored on the 31st
// still fires in February. time.Date normalises month 13 into next January.
func monthDay(year int, month time.Month, day int, atMinute int64, loc *time.Location) time.Time {
	if last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day(); day > last {
		day = last
	}
	return time.Date(year, month, day, int(atMinute/60), int(atMinute%60), 0, 0, loc)
}
