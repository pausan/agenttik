package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// openProvider's first turn streams whatever the test writes to events, so a
// test decides when replies end and prompts are taken. Later turns end at
// once.
type openProvider struct {
	fakeProvider
	runs    atomic.Int32
	events  chan agent.Event
	prompts chan string // what the first turn was handed
	refuse  atomic.Bool // FollowUp.Send fails, as it does once stdin closed
	ctx     context.Context
}

func newOpenProvider() *openProvider {
	return &openProvider{events: make(chan agent.Event), prompts: make(chan string, 4)}
}

func (p *openProvider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if p.runs.Add(1) > 1 {
		done := make(chan agent.Event, 1)
		done <- agent.Event{Type: agent.EventDone}
		close(done)
		return done, nil
	}
	p.ctx = ctx
	out := make(chan agent.Event)
	go func() {
		defer close(out)
		for ev := range p.events {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (p *openProvider) followUp() *agent.FollowUp {
	return agent.NewFollowUp(func(prompt string) error {
		if p.refuse.Load() {
			return errors.New("turn has ended")
		}
		p.prompts <- prompt
		return nil
	})
}

func roles(t *testing.T, st *store.Store, sessionID string) []string {
	t.Helper()
	msgs, err := st.ListMessages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role + ":" + m.Content
	}
	return out
}

// A prompt sent while the turn waits on background tasks goes into that turn.
// It is stored when the provider starts on it, after the reply it waited
// behind, and the turn ends once the provider closes it.
func TestPromptJoinsATurnWaitingOnBackgroundTasks(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	first, err := r.Send(sess.ID, "build both games")
	if err != nil {
		t.Fatal(err)
	}
	if r.AcceptsPrompts(sess.ID) {
		t.Fatal("a turn takes prompts only once a reply leaves it open")
	}
	p.events <- agent.Event{Type: agent.EventText, Text: "Builds started."}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")

	joined, err := r.Send(sess.ID, "status?")
	if err != nil || joined.ID != first.ID {
		t.Fatalf("follow-up joined turn %+v, err %v; want turn %d", joined, err, first.ID)
	}
	if got := <-p.prompts; got != "status?" {
		t.Fatalf("provider was handed %q", got)
	}
	// Stored on acceptance, without a turn until the provider starts on it.
	if last := lastMessage(t, st, sess.ID); last.Content != "status?" || last.TurnID != 0 {
		t.Fatalf("accepted prompt stored as %+v", last)
	}
	if pending := r.PendingFollowUps(sess.ID); len(pending) != 1 {
		t.Fatalf("pending = %v", pending)
	}
	// A task notification is answered before the prompt is taken.
	p.events <- agent.Event{Type: agent.EventText, Text: "Game A built."}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	p.events <- agent.Event{Type: agent.EventFollowUp}
	p.events <- agent.Event{Type: agent.EventText, Text: "Game B still building."}
	p.events <- agent.Event{Type: agent.EventDone}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")

	want := []string{"user:build both games", "assistant:Builds started.", "assistant:Game A built.",
		"user:status?", "assistant:Game B still building."}
	if got := roles(t, st, sess.ID); len(got) != len(want) {
		t.Fatalf("messages = %q, want %q", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("messages = %q, want %q", got, want)
			}
		}
	}
	if turns, _ := st.ListTurns(sess.ID); len(turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(turns))
	}
	messages, _ := st.ListMessages(sess.ID)
	for _, m := range messages {
		if m.TurnID != first.ID {
			t.Errorf("message %q has turn %d", m.Content, m.TurnID)
		}
	}
	if r.AcceptsPrompts(sess.ID) {
		t.Error("an ended turn still takes prompts")
	}
}

func lastMessage(t *testing.T, st *store.Store, sessionID string) store.Message {
	t.Helper()
	messages, err := st.ListMessages(sessionID)
	if err != nil || len(messages) == 0 {
		t.Fatalf("messages = %v, err %v", messages, err)
	}
	return messages[len(messages)-1]
}

// A turn that ends before the provider starts on the prompts it accepted —
// stopped, quit, or the CLI gone — keeps them: they move to the end of the
// transcript without a turn, which marks them unprocessed. Send now loses its
// queue row only once the provider has the prompt.
func TestUntakenFollowUpsSurviveTheTurn(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")
	if _, err := r.Send(sess.ID, "status?"); err != nil {
		t.Fatal(err)
	}
	<-p.prompts
	queued, err := r.Enqueue(sess.ID, "deploy when ready")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := r.ForceQueued(sess.ID, queued[0].ID, Refuse)
	if err != nil || joined == nil || joined.Content != "deploy when ready" || joined.TurnID != 0 {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	<-p.prompts
	p.events <- agent.Event{Type: agent.EventText, Text: "Still working."}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")

	messages, err := st.ListMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range messages {
		got = append(got, fmt.Sprintf("%s:%s:%v", m.Role, m.Content, m.TurnID != 0))
	}
	want := []string{"user:build:true", "assistant:Still working.:true", "user:status?:false", "user:deploy when ready:false"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("messages = %q, want %q", got, want)
	}
	if left, _ := st.ListQueuedMessages(sess.ID); len(left) != 0 {
		t.Fatalf("queue = %+v", left)
	}
}

// After a provider outage agenttik retries the turn by itself, and prompts it
// accepted but never started on wait in the queue behind that retry.
func TestUntakenFollowUpsWaitBehindAnOutageRetry(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")
	if _, err := r.Send(sess.ID, "status?"); err != nil {
		t.Fatal(err)
	}
	<-p.prompts
	p.events <- agent.Event{Type: agent.EventError, Text: capacityFailure}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")

	queued, err := st.ListQueuedMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 || queued[0].Prompt != "build" || queued[0].RetryAt == 0 || queued[1].Prompt != "status?" {
		t.Fatalf("queue = %+v", queued)
	}
	for _, m := range mustMessages(t, st, sess.ID) {
		if m.Content == "status?" {
			t.Fatalf("requeued prompt still in the transcript: %+v", m)
		}
	}
}

func mustMessages(t *testing.T, st *store.Store, sessionID string) []store.Message {
	t.Helper()
	messages, err := st.ListMessages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

// A prompt the closing turn refuses leaves no trace of the attempt: Send now
// then starts it as a turn of its own, from its queue row.
func TestRefusedSendNowStartsTheNextTurn(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")
	queued, err := r.Enqueue(sess.ID, "deploy when ready")
	if err != nil {
		t.Fatal(err)
	}
	p.refuse.Store(true)
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(p.events)
	}()
	if joined, err := r.ForceQueued(sess.ID, queued[0].ID, Refuse); err != nil || joined != nil {
		t.Fatalf("joined %+v, err %v", joined, err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) && p.runs.Load() == 2 }, "the prompt's own turn")
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "its turn to end")
	var got []string
	for _, m := range mustMessages(t, st, sess.ID) {
		got = append(got, fmt.Sprintf("%s:%v", m.Content, m.TurnID != 0))
	}
	if fmt.Sprint(got) != fmt.Sprint([]string{"build:true", "deploy when ready:true"}) {
		t.Fatalf("messages = %q", got)
	}
}

// A prompt that reaches a turn just as it stops taking prompts waits for that
// turn to end and starts the next one.
func TestPromptMissingTheOpenTurnStartsTheNext(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	first, err := r.Send(sess.ID, "build")
	if err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")
	p.refuse.Store(true)
	go close(p.events)

	next, err := r.Send(sess.ID, "and now?")
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID {
		t.Fatal("the prompt joined a turn that had stopped taking prompts")
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the second turn to end")
	if turns, _ := st.ListTurns(sess.ID); len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
}

// Force send on a turn that takes prompts hands the queued prompt to it
// instead of cancelling it, which would kill its background tasks.
func TestForceQueuedJoinsAnOpenTurn(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	waitFor(t, func() bool { return r.AcceptsPrompts(sess.ID) }, "the turn to take prompts")
	queued, err := r.Enqueue(sess.ID, "deploy when ready")
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued %+v, err %v", queued, err)
	}
	if joined, err := r.ForceQueued(sess.ID, queued[0].ID, Refuse); err != nil || joined == nil {
		t.Fatalf("joined %v, err %v", joined, err)
	}
	if got := <-p.prompts; got != "deploy when ready" {
		t.Fatalf("provider was handed %q", got)
	}
	if p.ctx.Err() != nil {
		t.Fatal("force send cancelled a turn that takes prompts")
	}
	if left, _ := st.ListQueuedMessages(sess.ID); len(left) != 0 {
		t.Fatalf("still queued: %+v", left)
	}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
}

// The project's task rows hear when a turn's background work changes, and
// only then.
func TestProjectHearsBackgroundWork(t *testing.T) {
	p := newOpenProvider()
	r, _, sess := setup(t, p)
	events, unsubscribe := r.Hub().Subscribe(ProjectTopic(sess.ProjectID))
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "build"); err != nil {
		t.Fatal(err)
	}
	next := func() Event {
		t.Helper()
		for {
			select {
			case ev := <-events:
				if ev.Event.Type == EventSessionBackground {
					return ev
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no session_background event")
			}
		}
	}
	task := agent.BackgroundTask{ID: "b1", Kind: "shell", Status: "running"}
	p.events <- agent.BackgroundEvent(task)
	if ev := next(); ev.BackgroundTasks != 1 || ev.AcceptsPrompts {
		t.Fatalf("task started: %+v", ev)
	}
	p.events <- agent.BackgroundEvent(task) // unchanged: not repeated
	p.events <- agent.Event{Type: agent.EventDone, FollowUp: p.followUp()}
	if ev := next(); ev.BackgroundTasks != 1 || !ev.AcceptsPrompts {
		t.Fatalf("reply ended: %+v", ev)
	}
	if running, accepts := r.BackgroundWork(sess.ID); running != 1 || !accepts {
		t.Fatalf("BackgroundWork = %d, %v", running, accepts)
	}
	task.Status = "completed"
	p.events <- agent.BackgroundEvent(task)
	if ev := next(); ev.BackgroundTasks != 0 || !ev.AcceptsPrompts {
		t.Fatalf("task ended: %+v", ev)
	}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
}

// Two Send nows on one queued prompt — two windows, a retry — hand it to the
// turn once: the row is taken under the turn's lock, and the second finds it
// gone.
func TestSendNowTwiceHandsThePromptOverOnce(t *testing.T) {
	p := newOpenProvider()
	r, st, sess := setup(t, p)
	openOn(t, p, r, sess.ID)
	queued, err := r.Enqueue(sess.ID, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := r.ForceQueued(sess.ID, queued[0].ID, Refuse)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var failed []error
	for err := range errs {
		if err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) != 1 || !errors.Is(failed[0], store.ErrNotFound) {
		t.Fatalf("errors = %v, want one not found", failed)
	}
	got := 0
	for done := false; !done; {
		select {
		case <-p.prompts:
			got++
		case <-time.After(200 * time.Millisecond):
			done = true
		}
	}
	stored := 0
	for _, m := range mustMessages(t, st, sess.ID) {
		if m.Content == "deploy" {
			stored++
		}
	}
	if got != 1 || stored != 1 {
		t.Fatalf("provider handed %d copies, %d messages stored; want 1 and 1", got, stored)
	}
	if left, _ := st.ListQueuedMessages(sess.ID); len(left) != 0 {
		t.Fatalf("still queued: %+v", left)
	}
	close(p.events)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "the turn to end")
}
