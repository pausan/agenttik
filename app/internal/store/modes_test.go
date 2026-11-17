package store

import (
	"errors"
	"testing"
)

func TestModeTitlesAreUniqueIgnoringCase(t *testing.T) {
	s := testStore(t)
	review, err := s.CreateMode(" Review ", " Read first. ")
	must(t, err)
	if review.Title != "Review" || review.Body != "Read first." {
		t.Fatalf("mode = %+v", review)
	}
	if _, err := s.CreateMode("review", "x"); !errors.Is(err, ErrModeTitleTaken) {
		t.Fatalf("duplicate title: %v", err)
	}
	tdd, err := s.CreateMode("TDD", "Test first.")
	must(t, err)
	if err := s.UpdateMode(tdd.ID, "REVIEW", "x"); !errors.Is(err, ErrModeTitleTaken) {
		t.Fatalf("rename onto a taken title: %v", err)
	}
	modes, err := s.ListModes()
	must(t, err)
	if len(modes) != 2 || modes[0].Title != "Review" || modes[1].Title != "TDD" {
		t.Fatalf("modes = %+v", modes)
	}
}

func TestDeletingAModeUnlinksIt(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("p", t.TempDir())
	must(t, err)
	m, err := s.CreateMode("Review", "Read first.")
	must(t, err)
	must(t, s.SetProjectMode(p.ID, m.ID))
	sess := &Session{ID: "a", ProjectID: p.ID, Provider: "fake", ModeID: m.ID}
	must(t, s.CreateSession(sess))
	none := &Session{ID: "b", ProjectID: p.ID, Provider: "fake", ModeID: ModeNone}
	must(t, s.CreateSession(none))
	pin := &Schedule{ProjectID: p.ID, Prompt: "x", Every: EveryPinned, ModeID: m.ID}
	must(t, s.CreateSchedule(pin))

	must(t, s.DeleteMode(m.ID))
	got, err := s.GetProject(p.ID)
	must(t, err)
	a, err := s.GetSession("a")
	must(t, err)
	b, err := s.GetSession("b")
	must(t, err)
	j, err := s.GetSchedule(pin.ID)
	must(t, err)
	if got.ModeID != 0 || a.ModeID != ModeOfProject || b.ModeID != ModeNone || j.ModeID != ModeOfProject {
		t.Fatalf("project %d, task %d, none %d, pin %d", got.ModeID, a.ModeID, b.ModeID, j.ModeID)
	}
	if err := s.DeleteMode(m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestMoveProjectCarriesItsModes(t *testing.T) {
	source, target := testStore(t), testStore(t)
	moveFixture(t, source, "/source", "source-task")
	review, err := source.CreateMode("Review", "Source review.")
	must(t, err)
	tdd, err := source.CreateMode("TDD", "Test first.")
	must(t, err)
	_, err = source.CreateMode("Unused", "Stays.")
	must(t, err)
	must(t, source.SetProjectMode(1, review.ID))
	must(t, source.SetSessionMode("source-task", tdd.ID))
	must(t, source.SetScheduleMode(1, ModeNone))
	_, err = target.CreateMode("Other", "x")
	must(t, err)
	kept, err := target.CreateMode("review", "Target review.")
	must(t, err)

	id, err := source.MoveProjectTo(target, 1)
	must(t, err)
	p, err := target.GetProject(id)
	must(t, err)
	if p.ModeID != kept.ID {
		t.Fatalf("project mode = %d, want the target's own %d", p.ModeID, kept.ID)
	}
	sess, err := target.GetSession("source-task")
	must(t, err)
	moved, err := target.GetMode(sess.ModeID)
	must(t, err)
	if moved.Title != "TDD" || moved.Body != "Test first." {
		t.Fatalf("task mode = %+v", moved)
	}
	jobs, err := target.ListSchedules(ScheduleFilter{ProjectID: id})
	must(t, err)
	if len(jobs) != 1 || jobs[0].ModeID != ModeNone {
		t.Fatalf("schedules = %+v", jobs)
	}
	modes, err := target.ListModes()
	must(t, err)
	if len(modes) != 3 {
		t.Fatalf("target modes = %+v", modes)
	}
	if again, _ := target.GetMode(kept.ID); again.Body != "Target review." {
		t.Fatalf("target mode rewritten: %+v", again)
	}
}
