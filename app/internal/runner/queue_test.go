package runner

import (
	"errors"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestDeleteQueuedKeepsRunningTurnAndOtherPrompts(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{gate: make(chan struct{})})
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(sess.ID, "running"); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for range 3 {
		q, err := st.EnqueueMessage(sess.ID, "same text", sess.Provider, 0, sess.Model, sess.Effort)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, q.ID)
	}
	if err := r.DeleteQueued(sess.ID, ids[1]); err != nil {
		t.Fatal(err)
	}
	queued, err := st.ListQueuedMessages(sess.ID)
	if err != nil || len(queued) != 2 || queued[0].ID != ids[0] || queued[1].ID != ids[2] {
		t.Fatalf("remaining queue = %+v, %v", queued, err)
	}
	if !r.Running(sess.ID) {
		t.Fatal("deleting a queued prompt stopped the current turn")
	}
	messages, err := st.ListMessages(sess.ID)
	if err != nil || len(messages) != 1 || messages[0].Content != "running" {
		t.Fatalf("deletion changed transcript: %+v, %v", messages, err)
	}
	if err := r.DeleteQueued(sess.ID, ids[1]); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleting missing item = %v, want not found", err)
	}
}

func TestDeleteQueuedLastRetryReturnsToIdle(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{})
	q, err := st.RequeueMessage(store.QueuedMessage{SessionID: sess.ID, Prompt: "retry", Provider: sess.Provider,
		Model: sess.Model}, time.Now().Add(time.Hour).UnixMilli(), "rate limit")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionStatus(sess.ID, store.StatusWaiting); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteQueued(sess.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetSession(sess.ID)
	if err != nil || updated.Status != store.StatusIdle || updated.QueueCount != 0 {
		t.Fatalf("session after deleting retry = %+v, %v", updated, err)
	}
}

func TestDeleteQueuedClosesUnstartedScheduleRun(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{})
	job := &store.Schedule{ProjectID: sess.ProjectID, Title: "job", Prompt: "scheduled", Provider: "fake",
		Model: "m1", Every: store.EveryInterval, IntervalMinutes: 15, Remaining: 2}
	if err := st.CreateSchedule(job); err != nil {
		t.Fatal(err)
	}
	sess.ID, sess.ScheduleID = "scheduled", job.ID
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartScheduleRun(job.ID, sess.ID); err != nil {
		t.Fatal(err)
	}
	q, err := st.EnqueueMessage(sess.ID, job.Prompt, job.Provider, 0, job.Model, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteQueued(sess.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListScheduleRuns(job.ID, 0)
	if err != nil || len(runs) != 1 || runs[0].Status != store.RunInterrupted {
		t.Fatalf("schedule runs = %+v, %v", runs, err)
	}
	updated, err := st.GetSchedule(job.ID)
	if err != nil || updated.Remaining != 2 || updated.Running {
		t.Fatalf("schedule after deletion = %+v, %v", updated, err)
	}
}

func TestForceQueuedReturnsOnceThePromptHasStarted(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{gate: make(chan struct{})})
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(sess.ID, "running"); err != nil {
		t.Fatal(err)
	}
	q, err := st.EnqueueMessage(sess.ID, "forced", sess.Provider, 0, sess.Model, sess.Effort)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ForceQueued(sess.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	// No waiting: the answer means the new turn is already on record.
	messages, err := st.ListMessages(sess.ID)
	if err != nil || len(messages) != 2 || messages[1].Content != "forced" {
		t.Fatalf("transcript = %+v, %v", messages, err)
	}
	if !r.Running(sess.ID) {
		t.Fatal("forced prompt is not running")
	}
	if n, _ := st.QueueCount(sess.ID); n != 0 {
		t.Fatalf("queue count = %d", n)
	}
}

func TestForceQueuedReportsAFailedStart(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{})}
	r, st, sess := setup(t, fp)
	t.Cleanup(r.Shutdown)
	if _, err := r.Send(sess.ID, "running"); err != nil {
		t.Fatal(err)
	}
	q, err := st.EnqueueMessage(sess.ID, "forced", sess.Provider, 0, sess.Model, sess.Effort)
	if err != nil {
		t.Fatal(err)
	}
	fp.runErr = errors.New("provider would not start")
	if err := r.ForceQueued(sess.ID, q.ID); err == nil || err.Error() != "provider would not start" {
		t.Fatalf("force = %v, want the start failure", err)
	}
	if n, _ := st.QueueCount(sess.ID); n != 1 {
		t.Fatalf("queue count = %d, want the prompt kept", n)
	}
}
