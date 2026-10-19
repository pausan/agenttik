package runner

import (
	"errors"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestProjectQueueDrainsCurrentTaskBeforeTaskOrder(t *testing.T) {
	for _, trigger := range []string{"completion", "scheduler", "resume"} {
		t.Run(trigger, func(t *testing.T) {
			gate := make(chan struct{})
			r, st, current := setup(t, &fakeProvider{gate: gate})
			t.Cleanup(r.Shutdown)
			for _, id := range []string{"top", "middle"} {
				sess := *current
				sess.ID = id
				if err := st.CreateSession(&sess); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.ReorderSessions(current.ProjectID, []string{"top", "middle", current.ID}); err != nil {
				t.Fatal(err)
			}
			events, unsubscribe := r.Hub().Subscribe(ProjectTopic(current.ProjectID))
			defer unsubscribe()
			if _, err := r.Send(current.ID, "initial"); err != nil {
				t.Fatal(err)
			}
			// Queue in a different order from the sidebar, with more work for
			// the current task even though it is last in the project.
			for _, item := range []struct{ id, prompt string }{
				{"middle", "middle first"}, {"top", "top first"},
				{current.ID, "continue first"}, {current.ID, "continue second"},
			} {
				if _, err := r.Enqueue(item.id, item.prompt); err != nil {
					t.Fatal(err)
				}
			}
			nextStarted := func(want string) {
				t.Helper()
				for {
					select {
					case ev := <-events:
						if ev.Event.Type == "started" {
							if ev.Prompt != want {
								t.Fatalf("started %q in %s, want %q", ev.Prompt, ev.SessionID, want)
							}
							return
						}
					case <-time.After(2 * time.Second):
						t.Fatalf("timed out waiting for %q", want)
					}
				}
			}
			nextStarted("initial")
			if trigger == "scheduler" {
				// Let a different dispatch win the lock after completion.
				lock := r.dispatchLock(current.ProjectID)
				lock.Lock()
				gate <- struct{}{}
				waitFor(t, func() bool { return !r.Running(current.ID) }, "initial turn to release")
				r.dispatch(current.ProjectID)
				lock.Unlock()
			} else if trigger == "resume" {
				if err := r.SetProjectQueuePaused(current.ProjectID, true); err != nil {
					t.Fatal(err)
				}
				gate <- struct{}{}
				waitFor(t, func() bool { return !r.Running(current.ID) }, "paused turn to release")
				if err := r.SetProjectQueuePaused(current.ProjectID, false); err != nil {
					t.Fatal(err)
				}
			} else {
				gate <- struct{}{}
			}
			nextStarted("continue first")
			// Work accepted while the queue drains retains the same priority.
			if _, err := r.Enqueue(current.ID, "continue third"); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"continue second", "continue third", "top first", "middle first"} {
				gate <- struct{}{}
				nextStarted(want)
			}
			gate <- struct{}{}
			waitFor(t, func() bool { return !r.Busy() }, "project queue to finish")
		})
	}
}

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

func TestProjectPauseKeepsCurrentTurnAndBlocksAllQueuedTasks(t *testing.T) {
	gate := make(chan struct{})
	r, st, sess := setup(t, &fakeProvider{gate: gate})
	t.Cleanup(r.Shutdown)
	other := *sess
	other.ID = "other-task"
	if err := st.CreateSession(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Send(sess.ID, "running"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{sess.ID, other.ID} {
		if _, err := r.Enqueue(id, "waiting"); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SetProjectQueuePaused(sess.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	if !r.Running(sess.ID) {
		t.Fatal("pause cancelled the running turn")
	}
	close(gate)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "active turn to finish")
	// Exercise a later scheduler tick as well as turn completion.
	r.schedule(sess.ProjectID)
	for _, id := range []string{sess.ID, other.ID} {
		if n, err := st.QueueCount(id); err != nil || n != 1 {
			t.Fatalf("queue %s = %d, %v", id, n, err)
		}
	}
	// Prompts accepted while paused must wait too.
	if q, err := r.Enqueue(other.ID, "also waiting"); err != nil || len(q) != 2 {
		t.Fatalf("enqueue while paused = %+v, %v", q, err)
	}
	if err := r.SetProjectQueuePaused(sess.ProjectID, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		a, _ := st.QueueCount(sess.ID)
		b, _ := st.QueueCount(other.ID)
		return a == 0 && b == 0 && !r.Busy()
	}, "both task queues to drain after resume")
}

func TestCapacityWaitHoldsProjectQueue(t *testing.T) {
	gate := make(chan struct{})
	r, st, current := setup(t, &fakeProvider{gate: gate})
	t.Cleanup(r.Shutdown)
	for _, id := range []string{"active", "other"} {
		sess := *current
		sess.ID, sess.Model = id, "m2"
		if err := st.CreateSession(&sess); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Send("active", "already running"); err != nil {
		t.Fatal(err)
	}
	retry, err := st.RequeueMessage(store.QueuedMessage{SessionID: current.ID, Prompt: "retry",
		Provider: current.Provider, Model: current.Model, CreatedAt: time.Now().UnixMilli()},
		time.Now().Add(time.Hour).UnixMilli(), capacityFailure)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionStatus(current.ID, store.StatusWaiting); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, prompt string }{
		{"active", "after active"}, {"other", "forced"}, {current.ID, "follow-up"},
	} {
		if _, err := r.Enqueue(item.id, item.prompt); err != nil {
			t.Fatal(err)
		}
	}
	if !r.Running("active") {
		t.Fatal("capacity wait stopped an already running task")
	}
	gate <- struct{}{}
	waitFor(t, func() bool { return !r.Running("active") }, "already running task to finish")
	r.schedule(current.ProjectID)
	if r.Busy() {
		t.Fatal("another queued task started during capacity wait")
	}

	otherQueue, err := st.ListQueuedMessages("other")
	if err != nil || len(otherQueue) != 1 {
		t.Fatalf("other model's queue = %+v, %v", otherQueue, err)
	}
	if err := r.ForceQueued("other", otherQueue[0].ID); err != nil {
		t.Fatal(err)
	}
	if !r.Running("other") {
		t.Fatal("explicitly forced task did not run")
	}
	if _, err := r.Enqueue("other", "after forced"); err != nil {
		t.Fatal(err)
	}
	gate <- struct{}{}
	waitFor(t, func() bool { return !r.Running("other") }, "forced task to finish")
	r.schedule(current.ProjectID)
	if r.Busy() {
		t.Fatal("forced task completion released the capacity hold")
	}

	p, err := st.CreateProject("independent", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := *current
	elsewhere.ID, elsewhere.ProjectID = "elsewhere", p.ID
	if err := st.CreateSession(&elsewhere); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Enqueue(elsewhere.ID, "other project"); err != nil {
		t.Fatal(err)
	}
	if !r.Running(elsewhere.ID) {
		t.Fatal("capacity wait blocked another project")
	}
	gate <- struct{}{}
	waitFor(t, func() bool { return !r.Running(elsewhere.ID) }, "other project to finish")

	// Expire the retry. The held task must regain the project ahead of the
	// parallel task that most recently finished, then drain its own queue.
	if err := st.RemoveQueuedMessage(retry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RequeueMessage(*retry, time.Now().Add(-time.Second).UnixMilli(), capacityFailure); err != nil {
		t.Fatal(err)
	}
	r.schedule(current.ProjectID)
	if !r.Running(current.ID) || r.Running("active") || r.Running("other") {
		t.Fatal("recovery did not start the held task first")
	}
	gate <- struct{}{}
	waitFor(t, func() bool {
		messages, _ := st.ListMessages(current.ID)
		return len(messages) == 2 && messages[1].Content == "follow-up"
	}, "held task's follow-up to start")
	for _, id := range []string{"active", "other"} {
		if n, err := st.QueueCount(id); err != nil || n != 1 {
			t.Fatalf("%s queue = %d, %v; wanted held task to keep priority", id, n, err)
		}
	}
	close(gate)
	waitFor(t, func() bool {
		a, _ := st.QueueCount("active")
		b, _ := st.QueueCount("other")
		return a == 0 && b == 0 && !r.Busy()
	}, "project queue to drain after recovery")
}

func TestRemovingRetryReleasesProjectQueue(t *testing.T) {
	for _, action := range []string{"stop", "delete"} {
		t.Run(action, func(t *testing.T) {
			r, st, current := setup(t, &fakeProvider{})
			t.Cleanup(r.Shutdown)
			other := *current
			other.ID = "other"
			if err := st.CreateSession(&other); err != nil {
				t.Fatal(err)
			}
			retry, err := st.RequeueMessage(store.QueuedMessage{SessionID: current.ID, Prompt: "retry",
				Provider: current.Provider, Model: current.Model}, time.Now().Add(time.Hour).UnixMilli(), capacityFailure)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Enqueue(other.ID, "next task"); err != nil {
				t.Fatal(err)
			}
			if action == "stop" {
				err = r.Stop(current.ID)
			} else {
				err = r.DeleteQueued(current.ID, retry.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, func() bool {
				messages, _ := st.ListMessages(other.ID)
				return len(messages) == 1 && !r.Running(other.ID)
			}, "queue to resume after removing capacity hold")
		})
	}
}

func TestProjectPauseDoesNotBlockOtherProjects(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{})
	t.Cleanup(r.Shutdown)
	if err := r.SetProjectQueuePaused(sess.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Enqueue(sess.ID, "paused"); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject("other", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other := *sess
	other.ID, other.ProjectID = "other-project-task", p.ID
	if err := st.CreateSession(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Enqueue(other.ID, "run elsewhere"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		messages, _ := st.ListMessages(other.ID)
		return len(messages) > 0 && !r.Running(other.ID)
	}, "other project to finish")
	if n, _ := st.QueueCount(sess.ID); n != 1 {
		t.Fatalf("paused queue = %d", n)
	}
}
