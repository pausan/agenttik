package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
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

func TestAutomaticSchedulesQueueAndSkipTheirOwnPendingRuns(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{})}
	r, st, original := setup(t, fp)
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(original.ID, "existing task"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, title := range []string{"A", "B"} {
		s := &store.Schedule{ProjectID: original.ProjectID, Title: title, Prompt: title, Provider: "fake",
			Model: "m1", Effort: "high", Permission: "workspace",
			Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 2, NextRunAt: now.UnixMilli()}
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
		if err != nil || len(runs) != 1 || r.Running(runs[0].SessionID) {
			t.Fatalf("job %s did not wait behind the existing task: %+v, %v", job.Title, runs, err)
		}
		queued, err := st.ListQueuedMessages(runs[0].SessionID)
		if err != nil || len(queued) != 1 || queued[0].Prompt != job.Prompt || queued[0].Provider != job.Provider ||
			queued[0].Model != job.Model || queued[0].Effort != job.Effort {
			t.Fatalf("job %s queue = %+v, %v", job.Title, queued, err)
		}
		if hasTurns, err := st.HasTurns(runs[0].SessionID); err != nil || hasTurns {
			t.Fatalf("job %s started a turn before the project was free: %v", job.Title, err)
		}
	}
	r.tickSchedules(now.Add(15 * time.Minute))
	for _, job := range jobs {
		runs, err := st.ListScheduleRuns(job.ID, 0)
		if err != nil || len(runs) != 2 || runs[0].Status != store.RunSkipped {
			t.Fatalf("job %s did not skip its queued run: %+v, %v", job.Title, runs, err)
		}
		if count, err := st.QueueCount(runs[1].SessionID); err != nil || count != 1 {
			t.Fatalf("job %s queued %d prompts, want 1: %v", job.Title, count, err)
		}
	}
	firstRuns, _ := st.ListScheduleRuns(jobs[0].ID, 0)
	secondRuns, _ := st.ListScheduleRuns(jobs[1].ID, 0)
	first, second := firstRuns[1].SessionID, secondRuns[1].SessionID
	if err := st.ReorderSessions(original.ProjectID, []string{original.ID, first, second}); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(original.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Running(first) }, "first scheduled run")
	if r.Running(second) {
		t.Fatal("second schedule started while the first was running")
	}
	r.tickSchedules(now.Add(30 * time.Minute))
	for _, job := range jobs {
		runs, err := st.ListScheduleRuns(job.ID, 0)
		if err != nil || len(runs) != 2 || runs[0].Status != store.RunSkipped || runs[0].Count != 2 {
			t.Fatalf("job %s did not skip its running or queued run: %+v, %v", job.Title, runs, err)
		}
		updated, err := st.GetSchedule(job.ID)
		if err != nil || updated.Remaining != 2 || !updated.Running {
			t.Fatalf("pending job = %+v, %v", updated, err)
		}
	}
	if err := r.Stop(first); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Running(second) }, "second scheduled run")
	if r.Running(first) {
		t.Fatal("first schedule still running when the second started")
	}
	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(second) }, "scheduled runs to finish")
	for _, job := range jobs {
		updated, err := st.GetSchedule(job.ID)
		if err != nil || updated.Remaining != 1 || updated.Running {
			t.Fatalf("finished job = %+v, %v", updated, err)
		}
	}
}

func TestStoppingQueuedScheduleAllowsNextRun(t *testing.T) {
	r, st, original := setup(t, &fakeProvider{gate: make(chan struct{})})
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(original.ID, "existing task"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s := &store.Schedule{ProjectID: original.ProjectID, Title: "scheduled", Prompt: "scheduled", Provider: "fake",
		Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 2, NextRunAt: now.UnixMilli()}
	if err := st.CreateSchedule(s); err != nil {
		t.Fatal(err)
	}
	r.tickSchedules(now)
	runs, err := st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if err := r.Stop(runs[0].SessionID); err != nil {
		t.Fatal(err)
	}
	job, err := st.GetSchedule(s.ID)
	if err != nil || job.Running || job.Remaining != 2 {
		t.Fatalf("stopped job = %+v, %v", job, err)
	}
	runs, err = st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 1 || runs[0].Status != store.RunInterrupted {
		t.Fatalf("stopped runs = %+v, %v", runs, err)
	}
	r.tickSchedules(now.Add(15 * time.Minute))
	runs, err = st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 2 || runs[0].Status != store.RunRunning {
		t.Fatalf("next run = %+v, %v", runs, err)
	}
	if count, err := st.QueueCount(runs[0].SessionID); err != nil || count != 1 {
		t.Fatalf("next queue count = %d, %v", count, err)
	}
}

func TestAutomaticScheduleStartsWhenProjectIsIdle(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{})}
	r, st, original := setup(t, fp)
	t.Cleanup(r.Shutdown)
	now := time.Now()
	s := &store.Schedule{ProjectID: original.ProjectID, Title: "scheduled", Prompt: "scheduled", Provider: "fake",
		Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 1, NextRunAt: now.UnixMilli()}
	if err := st.CreateSchedule(s); err != nil {
		t.Fatal(err)
	}
	r.tickSchedules(now)
	runs, err := st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 1 || !r.Running(runs[0].SessionID) {
		t.Fatalf("idle project run = %+v, %v", runs, err)
	}
	if count, err := st.QueueCount(runs[0].SessionID); err != nil || count != 0 {
		t.Fatalf("started queue count = %d, %v", count, err)
	}
	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(runs[0].SessionID) }, "scheduled run to finish")
	job, err := st.GetSchedule(s.ID)
	if err != nil || job.Running || job.Remaining != 0 {
		t.Fatalf("finished job = %+v, %v", job, err)
	}
	r.tickSchedules(now.Add(15 * time.Minute))
	runs, err = st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 1 || runs[0].Status != store.RunDone {
		t.Fatalf("exhausted schedule runs = %+v, %v", runs, err)
	}
}

func TestQueuedScheduleStartupFailureReleasesSchedule(t *testing.T) {
	for _, provider := range []string{"missing", "fake"} {
		t.Run(provider, func(t *testing.T) {
			r, st, original := setup(t, &fakeProvider{runErr: errors.New("invalid configuration")})
			t.Cleanup(r.Shutdown)
			now := time.Now()
			s := &store.Schedule{ProjectID: original.ProjectID, Title: "scheduled", Prompt: "scheduled", Provider: provider,
				Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 1, NextRunAt: now.UnixMilli()}
			if err := st.CreateSchedule(s); err != nil {
				t.Fatal(err)
			}
			r.tickSchedules(now)
			job, err := st.GetSchedule(s.ID)
			if err != nil || job.Running {
				t.Fatalf("failed job = %+v, %v", job, err)
			}
			runs, err := st.ListScheduleRuns(s.ID, 0)
			if err != nil || len(runs) != 1 || runs[0].Status != store.RunError {
				t.Fatalf("failed runs = %+v, %v", runs, err)
			}
			r.tickSchedules(now.Add(15 * time.Minute))
			runs, err = st.ListScheduleRuns(s.ID, 0)
			if err != nil || len(runs) != 2 || runs[0].Status != store.RunError {
				t.Fatalf("next attempt = %+v, %v", runs, err)
			}
		})
	}
}

func TestScheduleDispatchDoesNotHoldProjectTransferLock(t *testing.T) {
	r, st, original := setup(t, &fakeProvider{gate: make(chan struct{})})
	t.Cleanup(r.Shutdown)
	now := time.Now()
	s := &store.Schedule{ProjectID: original.ProjectID, Title: "scheduled", Prompt: "scheduled", Provider: "fake",
		Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 1, NextRunAt: now.UnixMilli()}
	if err := st.CreateSchedule(s); err != nil {
		t.Fatal(err)
	}
	lock := r.dispatchLock(original.ProjectID)
	lock.Lock()
	locked := true
	defer func() {
		if locked {
			lock.Unlock()
		}
	}()
	ticked := make(chan struct{})
	go func() {
		r.tickSchedules(now)
		close(ticked)
	}()
	waitFor(t, func() bool {
		runs, err := st.ListScheduleRuns(s.ID, 0)
		if err != nil || len(runs) != 1 {
			return false
		}
		count, err := st.QueueCount(runs[0].SessionID)
		return err == nil && count == 1
	}, "scheduled prompt to be queued")
	moved := make(chan error, 1)
	go func() { moved <- r.WithIdleProject(original.ProjectID, func() error { return nil }) }()
	select {
	case err := <-moved:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queue dispatch held the project transfer lock")
	}
	lock.Unlock()
	locked = false
	select {
	case <-ticked:
	case <-time.After(2 * time.Second):
		t.Fatal("schedule tick did not finish dispatching")
	}
}

func TestRestartResumesQueuedScheduleBeforeItsNextSlot(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{})}
	r, st, original := setup(t, fp)
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(original.ID, "existing task"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s := &store.Schedule{ProjectID: original.ProjectID, Title: "scheduled", Prompt: "scheduled", Provider: "fake",
		Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 1, NextRunAt: now.UnixMilli()}
	if err := st.CreateSchedule(s); err != nil {
		t.Fatal(err)
	}
	r.tickSchedules(now)
	r.Shutdown()
	if err := st.ResetRunningSessions(); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetRunningScheduleRuns(); err != nil {
		t.Fatal(err)
	}
	r = New(st, r.registry, NewHub())
	t.Cleanup(r.Shutdown)
	// The next slot is still in the future. A due-schedules tick must neither
	// enqueue a duplicate nor be necessary to start the persisted prompt.
	r.tickSchedules(now.Add(time.Minute))
	runs, err := st.ListScheduleRuns(s.ID, 0)
	if err != nil || len(runs) != 1 {
		t.Fatalf("restored runs = %+v, %v", runs, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.RunSchedules(ctx)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	waitFor(t, func() bool { return r.Running(runs[0].SessionID) }, "restored scheduled run")
	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(runs[0].SessionID) }, "restored run to finish")
	if fp.lastReq.Prompt != s.Prompt {
		t.Fatalf("restored prompt = %q, want %q", fp.lastReq.Prompt, s.Prompt)
	}
	job, err := st.GetSchedule(s.ID)
	if err != nil || job.Running || job.Remaining != 0 {
		t.Fatalf("restored finished job = %+v, %v", job, err)
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

func TestFinishConditionGoesFirstAndPausesTheJob(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reply  string
		status string
		paused bool
	}{
		{"condition false", "processed one line", store.RunDone, false},
		{"condition true", finishMarker, store.RunFinished, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakeProvider{script: []agent.Event{{Type: agent.EventText, Text: tc.reply}}}
			r, st, original := setup(t, fp)
			t.Cleanup(r.Shutdown)
			now := time.Now()
			s := &store.Schedule{ProjectID: original.ProjectID, Title: "lines", Prompt: "process one line",
				Provider: "fake", FinishCondition: "the file is empty", Every: store.EveryInterval,
				IntervalMinutes: 15, Remaining: -1, NextRunAt: now.UnixMilli()}
			if err := st.CreateSchedule(s); err != nil {
				t.Fatal(err)
			}
			r.tickSchedules(now)
			runs, err := st.ListScheduleRuns(s.ID, 0)
			if err != nil || len(runs) != 1 {
				t.Fatalf("runs = %+v, %v", runs, err)
			}
			waitFor(t, func() bool { return !r.Running(runs[0].SessionID) }, "scheduled run to finish")

			if !strings.HasPrefix(fp.lastReq.Prompt, "Before anything else, check this finish condition:\n\nthe file is empty") ||
				!strings.HasSuffix(fp.lastReq.Prompt, "process one line") {
				t.Fatalf("prompt = %q", fp.lastReq.Prompt)
			}
			job, err := st.GetSchedule(s.ID)
			if err != nil || job.Paused != tc.paused {
				t.Fatalf("job = %+v, %v; want paused %v", job, err, tc.paused)
			}
			runs, _ = st.ListScheduleRuns(s.ID, 0)
			if runs[0].Status != tc.status {
				t.Fatalf("run status = %q, want %q", runs[0].Status, tc.status)
			}
		})
	}
}

func TestSchedulePromptWithoutConditionIsUnchanged(t *testing.T) {
	if got := schedulePrompt(&store.Schedule{Prompt: "do it", FinishCondition: "  "}); got != "do it" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestSaysFinishedNeedsTheMarkerOnItsOwnLine(t *testing.T) {
	for reply, want := range map[string]bool{
		finishMarker:             true,
		"`" + finishMarker + "`": true,
		"The file is empty.\n\n**" + finishMarker + "**":        true,
		"I will reply with only " + finishMarker + " when done": false,
		"processed line 3": false,
	} {
		if got := saysFinished(reply); got != want {
			t.Errorf("saysFinished(%q) = %v, want %v", reply, got, want)
		}
	}
}
