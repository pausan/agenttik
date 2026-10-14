package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCalendarMigrationPreservesExistingSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.db")
	db := rawDatabase(t, path)
	at := slices.IndexFunc(migrations, func(sql string) bool { return strings.Contains(sql, "ADD COLUMN day_interval") })
	if at < 0 {
		t.Fatal("calendar migration missing")
	}
	if err := applyMigrations(db, 0, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO projects(id,name,path,created_at) VALUES(1,'project','/project',123);
INSERT INTO schedules(id,project_id,prompt,provider,model,every,at_minute,anchor_at,next_run_at,created_at)
VALUES(1,1,'morning check','fake','m','day',540,123,456,123);`)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, err := st.GetSchedule(1)
	if err != nil || s.Every != EveryDay || s.AtMinute != 540 || s.AnchorAt != 123 || s.NextRunAt != 456 || s.DayInterval != 1 || s.WeekdayMask != 0 {
		t.Fatalf("upgraded schedule = %+v, %v", s, err)
	}
}

func TestResetScheduleRunsPreservesQueuedRuns(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("project", "/project")
	must(t, err)
	for _, queued := range []bool{false, true} {
		job := &Schedule{ProjectID: p.ID, Provider: "fake", Every: EveryInterval, Remaining: 2, NextRunAt: 1}
		must(t, s.CreateSchedule(job))
		id := "running"
		if queued {
			id = "queued"
		}
		must(t, s.CreateSession(&Session{ID: id, ProjectID: p.ID, Provider: "fake", ScheduleID: job.ID}))
		_, err := s.StartScheduleRun(job.ID, id)
		must(t, err)
		if queued {
			_, err := s.EnqueueMessage(id, "scheduled", "fake", 0, "m1", "high")
			must(t, err)
		}
	}
	must(t, s.ResetRunningSessions())
	must(t, s.ResetRunningScheduleRuns())
	due, err := s.DueSchedules(1)
	must(t, err)
	if len(due) != 2 {
		t.Fatalf("due schedules = %+v", due)
	}
	for _, job := range due {
		runs, err := s.ListScheduleRuns(job.ID, 0)
		must(t, err)
		want := RunInterrupted
		if runs[0].SessionID == "queued" {
			want = RunRunning
		}
		if runs[0].Status != want || job.Running != (want == RunRunning) || job.Remaining != 2 {
			t.Fatalf("reset job = %+v, run = %+v; want %s", job, runs[0], want)
		}
	}
}
