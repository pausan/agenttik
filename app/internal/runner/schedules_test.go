package runner

import (
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

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
