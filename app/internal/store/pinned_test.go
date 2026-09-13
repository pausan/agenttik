package store

import (
	"testing"
	"time"
)

func TestPinnedPromptsStayFirstAndNeverBecomeDue(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("pins", t.TempDir())
	must(t, err)
	regular := &Schedule{ProjectID: p.ID, Prompt: "scheduled", Every: EveryInterval, Remaining: -1, NextRunAt: 1}
	must(t, s.CreateSchedule(regular))
	pinned := &Schedule{ProjectID: p.ID, Prompt: "saved", Every: EveryPinned, Remaining: -1, NextRunAt: 1}
	must(t, s.CreateSchedule(pinned))
	due, err := s.DueSchedules(time.Now().UnixMilli())
	must(t, err)
	if len(due) != 1 || due[0].ID != regular.ID {
		t.Fatalf("due = %+v", due)
	}
	list, err := s.ListSchedules(ScheduleFilter{ProjectID: p.ID})
	must(t, err)
	if len(list) != 2 || list[0].ID != pinned.ID {
		t.Fatalf("list = %+v", list)
	}
	must(t, s.SetSchedulePaused(pinned.ID, true))
	list, err = s.ListSchedules(ScheduleFilter{Since: time.Now().Add(time.Hour).UnixMilli()})
	must(t, err)
	found := false
	for _, item := range list {
		if item.ID == pinned.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("time filter hid pinned prompt")
	}
}
