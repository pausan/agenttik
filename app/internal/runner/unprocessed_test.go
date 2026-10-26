package runner

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// transcript lists a session's messages as content and whether a turn has it.
func transcript(t *testing.T, st *store.Store, sessionID string) string {
	t.Helper()
	var out []string
	for _, m := range mustMessages(t, st, sessionID) {
		out = append(out, fmt.Sprintf("%s:%v", m.Content, m.TurnID != 0))
	}
	return fmt.Sprint(out)
}

// A prompt the agent never processed is sent again as itself: the message
// moves below the rest of the transcript and is the new turn's prompt, and the
// started event says which row it was.
func TestSendUnprocessedStartsATurn(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{script: []agent.Event{
		{Type: agent.EventText, Text: "B is next."}, {Type: agent.EventDone}}})
	events, unsubscribe := r.Hub().Subscribe(sess.ID)
	defer unsubscribe()
	// What Stop leaves of two queued prompts.
	first, err := st.AddMessage(sess.ID, 0, store.RoleUser, "Is game B next?")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(sess.ID, 0, store.RoleUser, "And game C?"); err != nil {
		t.Fatal(err)
	}

	turn, joined, err := r.SendUnprocessed(sess.ID, first.ID, Refuse)
	if err != nil || turn == nil || joined != nil {
		t.Fatalf("turn %+v joined %+v, err %v", turn, joined, err)
	}
	for started := false; !started; {
		select {
		case ev := <-events:
			if started = ev.Event.Type == "started"; started && (ev.Message == nil || ev.Message.TurnID != turn.ID ||
				ev.Message.Content != "Is game B next?" || ev.Replaces != first.ID) {
				t.Fatalf("started with %+v, replacing %d", ev.Message, ev.Replaces)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no started event")
		}
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
	if got, want := transcript(t, st, sess.ID), "[And game C?:false Is game B next?:true B is next.:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}
}

// Into a turn that waits on background tasks, a prompt sent again goes as one
// typed then does: held until the agent starts on it, at the end of the
// transcript, then taken into the turn below the reply it waited behind.
func TestSendUnprocessedJoinsATurnTakingPrompts(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	stopped, err := st.AddMessage(sess.ID, 0, store.RoleUser, "Is game B next?")
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.Send(sess.ID, "build")
	if err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")

	turn, joined, err := r.SendUnprocessed(sess.ID, stopped.ID, Refuse)
	if err != nil || joined == nil || turn.ID != first.ID || joined.TurnID != 0 || joined.ID == stopped.ID {
		t.Fatalf("turn %+v joined %+v, err %v", turn, joined, err)
	}
	if got := <-p.prompts; got != "Is game B next?" {
		t.Fatalf("provider was handed %q", got)
	}
	if got, want := transcript(t, st, sess.ID), "[build:true Is game B next?:false]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}
	if pending := r.PendingFollowUps(sess.ID); len(pending) != 1 || pending[0] != joined.ID {
		t.Fatalf("pending = %v", pending)
	}
	// The turn has it now, so it is neither sent nor queued again.
	if _, _, err := r.SendUnprocessed(sess.ID, joined.ID, Refuse); !errors.Is(err, ErrNotUnprocessed) {
		t.Fatalf("sending it twice: %v", err)
	}
	if _, err := r.QueueUnprocessed(sess.ID, joined.ID); !errors.Is(err, ErrNotUnprocessed) {
		t.Fatalf("queueing a prompt the turn holds: %v", err)
	}

	p.events <- agent.Event{Type: agent.EventText, Text: "Game A built."}
	p.events <- agent.Event{Type: agent.EventFollowUp}
	p.events <- agent.Event{Type: agent.EventText, Text: "B is next."}
	p.events <- agent.Event{Type: agent.EventDone}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
	want := "[build:true Game A built.:true Is game B next?:true B is next.:true]"
	if got := transcript(t, st, sess.ID); got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}
}

// A turn busy with something else takes no prompt, and the message stays
// where it is. Queued instead, it leaves the transcript for the end of the
// queue, and comes back to the transcript when it runs.
func TestQueueUnprocessedWaitsBehindTheQueue(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	stopped, err := st.AddMessage(sess.ID, 0, store.RoleUser, "Is game B next?")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.SendUnprocessed(sess.ID, stopped.ID, Refuse); !errors.Is(err, ErrBusy) {
		t.Fatalf("sent into a busy turn: %v", err)
	}
	if got, want := transcript(t, st, sess.ID), "[Is game B next?:false build:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}

	if _, err := r.Enqueue(sess.ID, "deploy"); err != nil {
		t.Fatal(err)
	}
	queued, err := r.QueueUnprocessed(sess.ID, stopped.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 || queued[0].Prompt != "deploy" || queued[1].Prompt != "Is game B next?" ||
		queued[1].Model != "m1" || queued[1].Effort != "high" {
		t.Fatalf("queue = %+v", queued)
	}
	if got, want := transcript(t, st, sess.ID), "[build:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}

	close(p.events)
	waitFor(t, func() bool {
		left, _ := st.ListQueuedMessages(sess.ID)
		return !r.Running(sess.ID) && len(left) == 0 && p.runs.Load() == 3
	}, "the queue to run")
	if got, want := transcript(t, st, sess.ID), "[build:true deploy:true Is game B next?:true]"; got != want {
		t.Fatalf("messages = %s, want %s", got, want)
	}
}

// Only a prompt of yours the agent never processed can be sent or queued
// again: not one that ran, not the agent's, not another task's, and not one a
// spawned task copied from its thread, which ran there.
func TestUnprocessedRefusesOtherMessages(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{script: []agent.Event{{Type: agent.EventDone}}})
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
	ran := lastMessage(t, st, sess.ID)
	reply, err := st.AddMessage(sess.ID, 0, store.RoleAssistant, "Built.")
	if err != nil {
		t.Fatal(err)
	}
	// A task is spawned from a thread after the messages it copies, as a click
	// after them is.
	time.Sleep(5 * time.Millisecond)
	spawned :=&store.Session{ID: "spawned", ProjectID: sess.ProjectID, Provider: "fake", Model: "m1", OriginID: "thread"}
	if err := st.CreateSession(spawned); err != nil {
		t.Fatal(err)
	}
	if err := st.CopyMessages(sess.ID, spawned.ID); err != nil {
		t.Fatal(err)
	}
	copied := mustMessages(t, st, spawned.ID)[0]

	for _, tc := range []struct {
		session string
		id      int64
		want    error
	}{
		{sess.ID, ran.ID, ErrNotUnprocessed},
		{sess.ID, reply.ID, ErrNotUnprocessed},
		{spawned.ID, copied.ID, ErrNotUnprocessed},
		{spawned.ID, ran.ID, store.ErrNotFound},
		{sess.ID, 99999, store.ErrNotFound},
	} {
		if _, _, err := r.SendUnprocessed(tc.session, tc.id, Refuse); !errors.Is(err, tc.want) {
			t.Errorf("send %s/%d: %v, want %v", tc.session, tc.id, err, tc.want)
		}
		if _, err := r.QueueUnprocessed(tc.session, tc.id); !errors.Is(err, tc.want) {
			t.Errorf("queue %s/%d: %v, want %v", tc.session, tc.id, err, tc.want)
		}
	}
	if turns, _ := st.ListTurns(sess.ID); len(turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(turns))
	}
}

// A prompt queued or edited away just as its turn starts leaves no turn
// behind.
func TestSentPromptGoneLeavesNoTurn(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{script: []agent.Event{{Type: agent.EventDone}}})
	if _, _, err := r.send(store.QueuedMessage{SessionID: sess.ID, Prompt: "gone"}, 99999, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("send = %v", err)
	}
	if turns, _ := st.ListTurns(sess.ID); len(turns) != 0 || r.Running(sess.ID) {
		t.Fatalf("turns = %+v, running %v", turns, r.Running(sess.ID))
	}
}
