package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNextQueuedRunPrefersCurrentTaskThenProjectOrder(t *testing.T) {
	st := testStore(t)
	p, err := st.CreateProject("queue", t.TempDir())
	must(t, err)
	other, err := st.CreateProject("other", t.TempDir())
	must(t, err)
	for _, id := range []string{"top", "middle", "current"} {
		must(t, st.CreateSession(&Session{ID: id, ProjectID: p.ID, Provider: "fake", Model: "m1"}))
	}
	must(t, st.CreateSession(&Session{ID: "elsewhere", ProjectID: other.ID, Provider: "fake", Model: "m1"}))
	must(t, st.ReorderSessions(p.ID, []string{"top", "middle", "current"}))
	// Acceptance order across tasks does not set their priority.
	for _, id := range []string{"middle", "top", "current", "elsewhere"} {
		_, err := st.EnqueueMessage(id, id+" first", "fake", 0, "m1", "")
		must(t, err)
	}
	_, err = st.EnqueueMessage("current", "current second", "fake", 0, "m1", "")
	must(t, err)
	now := time.Now().UnixMilli()
	check := func(preferred, want string) *QueuedMessage {
		t.Helper()
		sess, q, err := st.NextQueuedRun(p.ID, preferred, now)
		if err != nil || sess == nil || q == nil || sess.ID != want || q.SessionID != want {
			t.Fatalf("next with preference %q = %+v, %+v, %v; want %s", preferred, sess, q, err, want)
		}
		return q
	}
	for _, preferred := range []string{"", "deleted", "elsewhere"} {
		check(preferred, "top")
	}
	for _, want := range []string{"current first", "current second"} {
		q := check("current", "current")
		if q.Prompt != want {
			t.Fatalf("current prompt = %q, want %q", q.Prompt, want)
		}
		must(t, st.RemoveQueuedMessage(q.ID))
	}
	check("current", "top")
	// A provider retry delay still lets ready work in other tasks run.
	_, err = st.RequeueMessage(QueuedMessage{SessionID: "current", Prompt: "retry", Provider: "fake", Model: "m1"},
		now+time.Hour.Milliseconds(), "rate limit")
	must(t, err)
	q := check("current", "top")
	must(t, st.RemoveQueuedMessage(q.ID))
	check("current", "middle")
}

func TestRetryBlocksLaterPromptsInItsTask(t *testing.T) {
	st := testStore(t)
	p, err := st.CreateProject("queue", t.TempDir())
	must(t, err)
	for _, id := range []string{"waiting", "ready"} {
		must(t, st.CreateSession(&Session{ID: id, ProjectID: p.ID, Provider: "fake", Model: "m1"}))
	}
	later, err := st.EnqueueMessage("waiting", "later", "fake", 0, "m1", "")
	must(t, err)
	// Even in the same acceptance millisecond, the reinserted retry must
	// keep its place ahead of a follow-up that now has a smaller row id.
	retry, err := st.RequeueMessage(QueuedMessage{SessionID: "waiting", Prompt: "retry",
		Provider: "fake", Model: "m1", CreatedAt: later.CreatedAt},
		later.CreatedAt+time.Minute.Milliseconds(), "model is at capacity")
	must(t, err)
	_, err = st.EnqueueMessage("ready", "other task", "fake", 0, "m1", "")
	must(t, err)
	now := later.CreatedAt
	q, err := st.NextQueuedMessage("waiting", now)
	if err != nil || q != nil {
		t.Fatalf("waiting task offered %+v, %v", q, err)
	}
	sess, q, err := st.NextQueuedRun(p.ID, "waiting", now)
	if err != nil || sess == nil || sess.ID != "ready" || q == nil || q.Prompt != "other task" {
		t.Fatalf("ready task = %+v, %+v, %v", sess, q, err)
	}
	projects, err := st.RetryProjects(retry.RetryAt - 1)
	if err != nil || len(projects) != 0 {
		t.Fatalf("early retry projects = %v, %v", projects, err)
	}
	projects, err = st.RetryProjects(retry.RetryAt)
	if err != nil || len(projects) != 1 || projects[0] != p.ID {
		t.Fatalf("due retry projects = %v, %v", projects, err)
	}
	q, err = st.NextQueuedMessage("waiting", retry.RetryAt)
	if err != nil || q == nil || q.ID != retry.ID {
		t.Fatalf("due prompt = %+v, %v", q, err)
	}
	must(t, st.RemoveQueuedMessage(retry.ID))
	q, err = st.NextQueuedMessage("waiting", retry.RetryAt)
	if err != nil || q == nil || q.ID != later.ID {
		t.Fatalf("follow-up = %+v, %v", q, err)
	}
}

func TestProjectQueuePauseSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paused.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject("paused", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if p.QueuePaused {
		t.Fatal("new project is paused")
	}
	if err := st.SetProjectQueuePaused(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err = st.GetProject(p.ID)
	if err != nil || !p.QueuePaused {
		t.Fatalf("reopened project = %+v, %v", p, err)
	}
}
