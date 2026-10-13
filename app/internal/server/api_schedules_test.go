package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestCalendarSchedulePersistenceAndEditing(t *testing.T) {
	s, st := newTestServer(t)
	s.registry = agent.NewRegistry(singleLoginProvider{name: "calendar-test"})
	p, err := st.CreateProject("calendar", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, every := range []string{store.EveryDays, store.EveryWeekdays} {
		t.Run(every, func(t *testing.T) {
			body := map[string]any{"project_id": p.ID, "provider": "calendar-test", "prompt": "Check project",
				"title": "Check project", "every": every, "day_interval": 3, "weekday_mask": 2 | 8 | 32,
				"at_minute": 9*60 + 30, "remaining": -1}
			resp := do(t, s, "POST", "/api/schedules", body)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create = %d", resp.StatusCode)
			}
			job := decode[scheduleDetail](t, resp).Schedule
			path := fmt.Sprintf("/api/schedules/%d", job.ID)
			if job.Every != every || job.DayInterval != 3 || job.WeekdayMask != 42 || job.AtMinute != 570 || job.NextRunAt <= time.Now().UnixMilli() {
				t.Fatalf("created = %+v", job)
			}
			// Editing the time preserves the date anchor and chosen days.
			body["at_minute"] = 10 * 60
			resp = do(t, s, "PATCH", path, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("edit = %d", resp.StatusCode)
			}
			updated := decode[store.Schedule](t, resp)
			if updated.AnchorAt != job.AnchorAt || updated.AtMinute != 600 || updated.DayInterval != 3 || updated.WeekdayMask != 42 {
				t.Fatalf("edited = %+v", updated)
			}
			// Resume uses the stored recurrence, not a new date anchor.
			if err := st.SetSchedulePaused(job.ID, true); err != nil {
				t.Fatal(err)
			}
			resp = do(t, s, "PATCH", path, map[string]any{"paused": false})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("resume = %d", resp.StatusCode)
			}
			saved, err := st.GetSchedule(job.ID)
			if err != nil || saved.Paused || saved.AnchorAt != job.AnchorAt || saved.NextRunAt != runner.FirstRun(saved, time.Now()).UnixMilli() {
				t.Fatalf("saved = %+v, %v", saved, err)
			}
			// Changing the N-day spacing starts from today.
			if every == store.EveryDays {
				body["day_interval"] = 4
				resp = do(t, s, "PATCH", path, body)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("change spacing = %d", resp.StatusCode)
				}
				updated = decode[store.Schedule](t, resp)
				if updated.DayInterval != 4 || updated.AnchorAt <= job.AnchorAt {
					t.Fatalf("new spacing = %+v", updated)
				}
			}
		})
	}
}

func TestCalendarScheduleValidation(t *testing.T) {
	s, st := newTestServer(t)
	s.registry = agent.NewRegistry(singleLoginProvider{name: "calendar-test"})
	p, err := st.CreateProject("calendar", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job := &store.Schedule{ProjectID: p.ID, Every: store.EveryDay, AtMinute: 540, Remaining: -1}
	if err := st.CreateSchedule(job); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/schedules/%d", job.ID)
	for _, body := range []map[string]any{
		{"every": "days"}, {"every": "days", "day_interval": -1}, {"every": "days", "day_interval": 36501},
		{"every": "weekdays"}, {"every": "weekdays", "weekday_mask": -1}, {"every": "weekdays", "weekday_mask": 128},
		{"every": "days", "day_interval": 2, "at_minute": 1440},
		{"every": "weekdays", "weekday_mask": 127, "at_minute": -1},
	} {
		resp := do(t, s, "PATCH", path, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("patch %+v = %d", body, resp.StatusCode)
		}
		body["project_id"], body["provider"], body["prompt"] = p.ID, "calendar-test", "Check project"
		resp = do(t, s, "POST", "/api/schedules", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("create %+v = %d", body, resp.StatusCode)
		}
	}
	saved, err := st.GetSchedule(job.ID)
	if err != nil || saved.Every != store.EveryDay || saved.AtMinute != 540 {
		t.Fatalf("invalid update changed recurrence: %+v, %v", saved, err)
	}
}
