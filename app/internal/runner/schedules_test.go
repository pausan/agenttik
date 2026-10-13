package runner

import (
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestCalendarRecurrences(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	parse := func(value string) time.Time {
		t.Helper()
		at, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	for _, tc := range []struct {
		name, every, anchor, now, want string
		days, mask                     int64
	}{
		{"daily before time", store.EveryDay, "2026-01-01 12:00", "2026-01-02 08:59", "2026-01-02 09:00", 0, 0},
		{"daily at time", store.EveryDay, "2026-01-01 12:00", "2026-01-02 09:00", "2026-01-03 09:00", 0, 0},
		{"weekly anchor", store.EveryWeek, "2026-01-05 12:00", "2026-01-06 10:00", "2026-01-12 09:00", 0, 0},
		{"monthly clamp", store.EveryMonth, "2026-01-31 12:00", "2026-02-01 10:00", "2026-02-28 09:00", 0, 0},
		{"N days first date", store.EveryDays, "2026-01-01 08:00", "2026-01-01 08:00", "2026-01-01 09:00", 3, 0},
		{"N days at time", store.EveryDays, "2026-01-01 08:00", "2026-01-01 09:00", "2026-01-04 09:00", 3, 0},
		{"N days between slots", store.EveryDays, "2026-01-01 08:00", "2026-01-02 08:00", "2026-01-04 09:00", 3, 0},
		{"N days crosses month", store.EveryDays, "2026-01-31 12:00", "2026-02-01 08:00", "2026-02-03 09:00", 3, 0},
		{"N days spring clock", store.EveryDays, "2026-03-28 08:00", "2026-03-29 08:00", "2026-03-30 09:00", 2, 0},
		{"N days autumn clock", store.EveryDays, "2026-10-24 08:00", "2026-10-25 08:00", "2026-10-26 09:00", 2, 0},
		{"N days long downtime", store.EveryDays, "2025-01-01 08:00", "2026-01-01 10:00", "2026-01-02 09:00", 3, 0},
		{"weekday today", store.EveryWeekdays, "2026-01-01 12:00", "2026-01-05 08:00", "2026-01-05 09:00", 0, 2 | 8 | 32},
		{"weekday at time", store.EveryWeekdays, "2026-01-01 12:00", "2026-01-05 09:00", "2026-01-07 09:00", 0, 2 | 8 | 32},
		{"weekday wraps week", store.EveryWeekdays, "2026-01-01 12:00", "2026-01-09 10:00", "2026-01-12 09:00", 0, 2 | 8 | 32},
		{"Sunday only", store.EveryWeekdays, "2026-01-01 12:00", "2026-01-11 09:00", "2026-01-18 09:00", 0, 1},
		{"weekdays spring clock", store.EveryWeekdays, "2026-01-01 12:00", "2026-03-28 10:00", "2026-03-29 09:00", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &store.Schedule{Every: tc.every, AnchorAt: parse(tc.anchor).UnixMilli(), AtMinute: 9 * 60,
				DayInterval: tc.days, WeekdayMask: tc.mask}
			now, want := parse(tc.now), parse(tc.want)
			if got := FirstRun(s, now); !got.Equal(want) {
				t.Fatalf("FirstRun = %s, want %s", got, want)
			}
			if got := AdvanceRun(s, parse(tc.anchor), now); !got.Equal(want) {
				t.Fatalf("AdvanceRun = %s, want %s", got, want)
			}
		})
	}
}

func TestIntervalScheduleDoesNotDriftOrReplay(t *testing.T) {
	s := &store.Schedule{Every: store.EveryInterval, IntervalMinutes: 15}
	prev := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for _, late := range []time.Duration{10 * time.Second, 3*time.Hour + 10*time.Second} {
		now := prev.Add(late)
		got := AdvanceRun(s, prev, now)
		if !got.After(now) || got.Sub(now) > 15*time.Minute || got.Minute()%15 != 0 || got.Second() != 0 {
			t.Fatalf("next slot = %s after %s", got, now)
		}
	}
}

func TestAutomaticSchedulesRunConcurrentlyAndSkipTheirOwnBusyRuns(t *testing.T) {
	r, st, original := setup(t, &fakeProvider{gate: make(chan struct{})})
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(original.ID, "existing task"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, title := range []string{"A", "B"} {
		s := &store.Schedule{ProjectID: original.ProjectID, Title: title, Prompt: title, Provider: "fake",
			Every: store.EveryInterval, IntervalMinutes: 15, Remaining: -1, NextRunAt: now.UnixMilli()}
		if err := st.CreateSchedule(s); err != nil {
			t.Fatal(err)
		}
	}
	r.tickSchedules(now)
	jobs, err := st.ListSchedules(store.ScheduleFilter{ProjectID: original.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		runs, err := st.ListScheduleRuns(job.ID, 0)
		if err != nil || len(runs) != 1 || !r.Running(runs[0].SessionID) {
			t.Fatalf("job %s did not start alongside the existing task: %+v, %v", job.Title, runs, err)
		}
	}
	r.tickSchedules(now.Add(15 * time.Minute))
	for _, job := range jobs {
		runs, err := st.ListScheduleRuns(job.ID, 0)
		if err != nil || len(runs) != 2 || runs[0].Status != store.RunSkipped {
			t.Fatalf("job %s did not skip its busy run: %+v, %v", job.Title, runs, err)
		}
	}
}

func TestFinishedScheduledRunStaysOpen(t *testing.T) {
	for _, tc := range []struct {
		name       string
		turnStatus string
		runStatus  string
		forced     bool
	}{
		{"scheduled success", "ok", store.RunDone, false},
		{"scheduled failure", "error", store.RunError, false},
		{"manual success", "ok", store.RunDone, true},
		{"manual failure", "error", store.RunError, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, st, original := setup(t, &fakeProvider{})
			t.Cleanup(r.Shutdown)
			schedule := &store.Schedule{ProjectID: original.ProjectID,
				Provider: "fake", Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 1}
			if err := st.CreateSchedule(schedule); err != nil {
				t.Fatalf("create schedule: %v", err)
			}
			sess := &store.Session{ID: "scheduled", ProjectID: original.ProjectID,
				Provider: "fake", ScheduleID: schedule.ID}
			if err := st.CreateSession(sess); err != nil {
				t.Fatalf("create session: %v", err)
			}
			if _, err := st.StartScheduleRun(schedule.ID, sess.ID); err != nil {
				t.Fatalf("start schedule run: %v", err)
			}
			if tc.forced {
				r.forcedScheduleRuns[sess.ID] = true
			}

			r.finishScheduledRun(sess, tc.turnStatus)

			updated, err := st.GetSession(sess.ID)
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if updated.DoneAt != 0 {
				t.Fatalf("finished task was automatically archived: done_at = %d", updated.DoneAt)
			}
			runs, err := st.ListScheduleRuns(schedule.ID, 0)
			if err != nil {
				t.Fatalf("list schedule runs: %v", err)
			}
			if len(runs) != 1 || runs[0].Status != tc.runStatus || runs[0].EndedAt == 0 {
				t.Fatalf("finished run = %+v, want one closed %s run", runs, tc.runStatus)
			}

			// A later prompt in the same task must not spend another run.
			r.finishScheduledRun(sess, "ok")
			job, err := st.GetSchedule(schedule.ID)
			if err != nil {
				t.Fatalf("get schedule: %v", err)
			}
			wantRemaining := int64(0)
			if tc.forced {
				wantRemaining = 1
			}
			if job.Running || job.Remaining != wantRemaining {
				t.Fatalf("running = %v, remaining = %d; want false, %d", job.Running, job.Remaining, wantRemaining)
			}
		})
	}
}
