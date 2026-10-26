package runner

import (
	"errors"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// openOn opens a turn on the session's choice, m1, that takes prompts.
func openOn(t *testing.T, p *openProvider, r *Runner, sessionID string) *store.Turn {
	t.Helper()
	turn, err := r.Send(sessionID, "build")
	if err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sessionID) }, "the turn to take prompts")
	return turn
}

func mismatched(t *testing.T, err error) *ChoiceError {
	t.Helper()
	var mismatch *ChoiceError
	if !errors.As(err, &mismatch) {
		t.Fatalf("err = %v, want a ChoiceError", err)
	}
	return mismatch
}

// A turn's process runs on the choice it was started on. A prompt sent after
// the task's model changed is refused rather than run on the old one, and
// nothing is kept; told to, it joins on the turn's choice, and the task keeps
// its new one for the turns after.
func TestSendRefusesAJoinOnAnotherChoice(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	first := openOn(t, p, r, sess.ID)
	if running := r.RunningChoice(sess.ID); running == nil || *running != (Choice{Provider: "fake", Model: "m1", Effort: "high"}) {
		t.Fatalf("running choice = %+v", running)
	}
	if err := st.SetSessionModel(sess.ID, "fake", 0, "m2", "high", false); err != nil {
		t.Fatal(err)
	}

	_, err := r.Send(sess.ID, "status?")
	if mismatch := mismatched(t, err); mismatch.Running.Model != "m1" || mismatch.Wanted.Model != "m2" {
		t.Fatalf("mismatch = %+v", mismatch)
	}
	if got, want := transcript(t, st, sess.ID), "[build:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}

	joined, err := r.SendWith(sess.ID, "status?", Join)
	if err != nil || joined.ID != first.ID {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	if got := <-p.prompts; got != "status?" {
		t.Fatalf("provider was handed %q", got)
	}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
	if got, _ := st.GetSession(sess.ID); got.Model != "m2" {
		t.Fatalf("session model = %q", got.Model)
	}
	if r.RunningChoice(sess.ID) != nil {
		t.Fatal("an ended turn still has a choice")
	}

	// Told to join a turn that has ended meanwhile, as when it ends with the
	// dialog open, the prompt has nothing to join and starts a turn on the
	// task's own choice.
	if _, err := r.SendWith(sess.ID, "next", Join); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the next turn to end")
	if turns, _ := st.ListTurns(sess.ID); len(turns) != 2 || turns[1].Model != "m2" {
		t.Fatalf("turns = %+v", turns)
	}
}

// Send now on a prompt queued for another choice than the turn's does not
// hand it to that turn. Refused, it stays queued; told to join, it runs on the
// turn's choice; told to stop, the turn goes, background tasks and all, and the
// prompt starts on its own choice.
func TestForceQueuedOnAnotherChoice(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	openOn(t, p, r, sess.ID)
	elsewhere, err := st.EnqueueMessage(sess.ID, "deploy", "fake", 0, "m2", "high")
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.ForceQueued(sess.ID, elsewhere.ID, Refuse)
	if mismatch := mismatched(t, err); mismatch.Wanted.Model != "m2" {
		t.Fatalf("mismatch = %+v", mismatch)
	}
	if left, _ := st.ListQueuedMessages(sess.ID); len(left) != 1 || p.ctx.Err() != nil {
		t.Fatalf("queue %+v; turn cancelled: %v", left, p.ctx.Err())
	}

	if joined, err := r.ForceQueued(sess.ID, elsewhere.ID, Join); err != nil || joined == nil {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	if got := <-p.prompts; got != "deploy" {
		t.Fatalf("provider was handed %q", got)
	}

	later, err := st.EnqueueMessage(sess.ID, "test", "fake", 0, "m2", "high")
	if err != nil {
		t.Fatal(err)
	}
	// The CLI exits once cancelled.
	go func() {
		<-p.ctx.Done()
		close(p.events)
	}()
	if joined, err := r.ForceQueued(sess.ID, later.ID, Stop); err != nil || joined != nil {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	waitFor(t, func() bool { return p.runs.Load() == 2 && !r.Running(sess.ID) }, "the prompt's own turn")
	turns, err := st.ListTurns(sess.ID)
	if err != nil || len(turns) != 2 || turns[1].Model != "m2" {
		t.Fatalf("turns = %+v, err %v", turns, err)
	}
	if left, _ := st.ListQueuedMessages(sess.ID); len(left) != 0 {
		t.Fatalf("still queued: %+v", left)
	}
}

// An unprocessed prompt sent into a turn on another choice than the task's is
// refused and stays where it is; told to, it joins on the turn's.
func TestSendUnprocessedOnAnotherChoice(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	stopped, err := st.AddMessage(sess.ID, 0, store.RoleUser, "Is game B next?")
	if err != nil {
		t.Fatal(err)
	}
	openOn(t, p, r, sess.ID)
	if err := st.SetSessionModel(sess.ID, "fake", 0, "m1", "low", false); err != nil {
		t.Fatal(err)
	}

	_, _, err = r.SendUnprocessed(sess.ID, stopped.ID, Refuse)
	if mismatch := mismatched(t, err); mismatch.Running.Effort != "high" || mismatch.Wanted.Effort != "low" {
		t.Fatalf("mismatch = %+v", mismatch)
	}
	if got, want := transcript(t, st, sess.ID), "[Is game B next?:false build:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}
	if _, joined, err := r.SendUnprocessed(sess.ID, stopped.ID, Join); err != nil || joined == nil {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	if got := <-p.prompts; got != "Is game B next?" {
		t.Fatalf("provider was handed %q", got)
	}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
}
