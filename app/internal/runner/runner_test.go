package runner

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// fakeProvider replays a fixed script and blocks until released, so tests can
// observe a turn while it is still running.
type fakeProvider struct {
	script  []agent.Event
	gate    chan struct{} // closed to let the script finish
	runErr  error         // returned instead of starting, for the failure paths
	lastReq agent.TurnRequest
}

func (f *fakeProvider) Name() string          { return "fake" }
func (f *fakeProvider) DisplayName() string   { return "Fake" }
func (f *fakeProvider) Models() []agent.Model { return []agent.Model{{ID: "m1", Label: "M1"}} }
func (f *fakeProvider) Efforts() []string     { return []string{"high"} }
func (f *fakeProvider) Available() error      { return nil }

func (f *fakeProvider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	f.lastReq = req
	if f.runErr != nil {
		return nil, f.runErr
	}
	ch := make(chan agent.Event)
	go func() {
		defer close(ch)
		for _, ev := range f.script {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		if f.gate != nil {
			select {
			case <-f.gate:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func setup(t *testing.T, fp agent.Provider) (*Runner, *store.Store, *store.Session) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	p, err := st.CreateProject("alpha", t.TempDir())
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sess := &store.Session{ID: "sess-1", ProjectID: p.ID, Provider: "fake",
		Model: "m1", Effort: "high", Permission: "workspace"}
	if err := st.CreateSession(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	r := New(st, agent.NewRegistry(fp), NewHub())
	return r, st, sess
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestTurnPersistsTranscriptAndMetrics(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{
		{Type: agent.EventSessionStarted, ProviderSessionID: "prov-1"},
		{Type: agent.EventText, Text: "thinking about it. "},
		{Type: agent.EventToolUse, Tool: &agent.ToolEvent{Name: "Read", Input: `{"path":"/x"}`}},
		{Type: agent.EventText, Text: "done"},
		{Type: agent.EventDone, Usage: &agent.Usage{InputTokens: 11, OutputTokens: 22,
			CacheReadTokens: 3, CacheWriteTokens: 4, CostUSD: 1.5,
			ContextTokens: 7, ContextWindow: 100,
			MainInputTokens: 8, MainOutputTokens: 20,
			SubagentInputTokens: 3, SubagentOutputTokens: 2,
			SubagentCount: 1, UsageBreakdown: true}},
	}}
	r, st, sess := setup(t, fp)

	if _, err := r.Send(sess.ID, "please look at x"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")

	msgs, err := st.ListMessages(sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	wantRoles := []string{store.RoleUser, store.RoleAssistant, store.RoleTool, store.RoleAssistant}
	if len(msgs) != len(wantRoles) {
		t.Fatalf("got %d messages, want %d: %+v", len(msgs), len(wantRoles), msgs)
	}
	for i, want := range wantRoles {
		if msgs[i].Role != want {
			t.Errorf("message %d role = %q, want %q", i, msgs[i].Role, want)
		}
	}
	// Prose is flushed before the tool call so ordering survives.
	if msgs[1].Content != "thinking about it. " || msgs[3].Content != "done" {
		t.Errorf("assistant text split wrongly: %q / %q", msgs[1].Content, msgs[3].Content)
	}

	stats, _ := st.SessionStats(sess.ID)
	if stats.Turns != 1 || stats.InputTokens != 11 || stats.OutputTokens != 22 ||
		stats.CacheReadTokens != 3 || stats.CacheWriteTokens != 4 || stats.CostUSD != 1.5 {
		t.Errorf("stats = %+v", stats)
	}
	if stats.ContextTokens != 7 || stats.ContextWindow != 100 ||
		stats.MainInputTokens != 8 || stats.SubagentInputTokens != 3 ||
		stats.SubagentCount != 1 || stats.UsageBreakdownTurns != 1 {
		t.Errorf("scoped stats = %+v", stats)
	}

	updated, _ := st.GetSession(sess.ID)
	if updated.ProviderSessionID != "prov-1" {
		t.Errorf("provider session id = %q, want prov-1", updated.ProviderSessionID)
	}
	if updated.Status != store.StatusIdle {
		t.Errorf("status = %q, want idle", updated.Status)
	}
	if updated.Title != "please look at x" {
		t.Errorf("title = %q", updated.Title)
	}
}

func TestSecondTurnResumes(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{
		{Type: agent.EventSessionStarted, ProviderSessionID: "prov-1"},
		{Type: agent.EventDone, Usage: &agent.Usage{}},
	}}
	r, _, sess := setup(t, fp)

	for i := 0; i < 2; i++ {
		if _, err := r.Send(sess.ID, "hi"); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")
	}
	if fp.lastReq.ProviderSessionID != "prov-1" {
		t.Errorf("second turn did not resume: %+v", fp.lastReq)
	}
}

func TestPromptUnarchivesTask(t *testing.T) {
	for _, queued := range []bool{false, true} {
		name := "send"
		if queued {
			name = "queue"
		}
		t.Run(name, func(t *testing.T) {
			fp := &fakeProvider{script: []agent.Event{{Type: agent.EventDone}}}
			r, st, sess := setup(t, fp)
			if err := st.SetSessionDone(sess.ID, true); err != nil {
				t.Fatal(err)
			}
			if queued {
				// Keep the prompt waiting to verify restoration happens on
				// submission, even before the scheduler can start its turn.
				r.active["other"] = activeTurn{projectID: sess.ProjectID}
				waiting, err := r.Enqueue(sess.ID, "continue")
				if err != nil {
					t.Fatal(err)
				}
				if len(waiting) != 1 {
					t.Fatalf("queued prompts = %d, want 1", len(waiting))
				}
			} else {
				ch, unsubscribe := r.Hub().Subscribe(sess.ID)
				defer unsubscribe()
				if _, err := r.Send(sess.ID, "continue"); err != nil {
					t.Fatal(err)
				}
				waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")
				waitFor(t, func() bool {
					select {
					case event := <-ch:
						if event.Event.Type == "started" {
							if event.Session == nil || event.Session.DoneAt != 0 {
								t.Fatalf("started event has archived session: %+v", event.Session)
							}
							return true
						}
					default:
					}
					return false
				}, "started event")
			}
			updated, err := st.GetSession(sess.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.DoneAt != 0 {
				t.Fatalf("task is still archived: done_at = %d", updated.DoneAt)
			}
		})
	}
}

func TestSecondPromptWhileRunningIsRejected(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{}),
		script: []agent.Event{{Type: agent.EventText, Text: "working"}}}
	r, st, sess := setup(t, fp)

	if _, err := r.Send(sess.ID, "one"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool {
		s, _ := st.GetSession(sess.ID)
		return s.Status == store.StatusRunning
	}, "session to be running")

	if _, err := r.Send(sess.ID, "two"); err != ErrBusy {
		t.Errorf("second send err = %v, want ErrBusy", err)
	}
	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")
}

func TestStopCancelsTurn(t *testing.T) {
	fp := &fakeProvider{gate: make(chan struct{}),
		script: []agent.Event{{Type: agent.EventText, Text: "working"}}}
	r, _, sess := setup(t, fp)

	if _, err := r.Send(sess.ID, "one"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool { return r.Running(sess.ID) }, "turn to start")
	if err := r.Stop(sess.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to stop")

	if err := r.Stop(sess.ID); err != ErrNotRunning {
		t.Errorf("stop when idle = %v, want ErrNotRunning", err)
	}
}

func TestStopPreservesUnstartedQueuedPromptsAfterRestart(t *testing.T) {
	r, st, sess := setup(t, &fakeProvider{})
	prompts := []string{"First prompt\nwith details", "Second prompt", "Second prompt"}
	for _, prompt := range prompts {
		if _, err := st.EnqueueMessage(sess.ID, prompt, sess.Provider, 0, sess.Model, sess.Effort); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(sess.ID); err != ErrNotRunning {
		t.Fatalf("second stop = %v", err)
	}
	path := filepath.Join(st.Dir(), "t.db")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	messages, err := reopened.ListMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != len(prompts) {
		t.Fatalf("messages = %+v, want %d saved prompts", messages, len(prompts))
	}
	for i, message := range messages {
		if message.Content != prompts[i] || message.Role != store.RoleUser || message.TurnID != 0 {
			t.Errorf("message %d = %+v", i, message)
		}
	}
	queued, err := reopened.ListQueuedMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 0 {
		t.Fatalf("queue = %+v", queued)
	}
	turns, err := reopened.ListTurns(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Fatalf("stopped prompts started turns: %+v", turns)
	}
}

func TestErrorEventMarksSessionFailed(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{
		{Type: agent.EventError, Text: "boom"},
		{Type: agent.EventDone, Usage: &agent.Usage{}},
	}}
	r, st, sess := setup(t, fp)

	if _, err := r.Send(sess.ID, "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")

	s, _ := st.GetSession(sess.ID)
	if s.Status != store.StatusError {
		t.Errorf("status = %q, want error", s.Status)
	}
	turns, _ := st.ListTurns(sess.ID)
	if len(turns) != 1 || turns[0].Status != "error" || turns[0].Error != "boom" {
		t.Errorf("turns = %+v", turns)
	}
}

func TestHubDeliversEventsToSubscribers(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{
		{Type: agent.EventText, Text: "hello"},
		{Type: agent.EventDone, Usage: &agent.Usage{}},
	}}
	r, _, sess := setup(t, fp)

	ch, unsub := r.Hub().Subscribe(sess.ID)
	defer unsub()

	if _, err := r.Send(sess.ID, "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}

	var texts string
	var sawStats bool
	timeout := time.After(2 * time.Second)
	for !sawStats {
		select {
		case ev := <-ch:
			if ev.Event.Type == agent.EventText {
				texts += ev.Event.Text
			}
			if ev.Stats != nil {
				sawStats = true
			}
		case <-timeout:
			t.Fatalf("timed out; texts so far %q", texts)
		}
	}
	if texts != "hello" {
		t.Errorf("streamed text = %q, want hello", texts)
	}
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("s")
	defer unsub()
	// Overflow the buffer without reading; publishing must not block.
	for i := 0; i < subscriberBuffer+10; i++ {
		h.Publish("s", Event{SessionID: "s"})
	}
	drained := 0
	for range ch {
		drained++
	}
	if drained > subscriberBuffer {
		t.Errorf("drained %d, want at most %d", drained, subscriberBuffer)
	}
}

// The project view watches one topic for the whole project, so the done event
// has to reach it as well as the session's own subscribers.
func TestDoneReachesProjectTopic(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{{Type: agent.EventDone, Usage: &agent.Usage{}}}}
	r, _, sess := setup(t, fp)

	ch, unsub := r.Hub().Subscribe(ProjectTopic(sess.ProjectID))
	defer unsub()

	if _, err := r.Send(sess.ID, "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("project topic closed before done")
			}
			if ev.Event.Type != agent.EventDone {
				continue // The project also receives started and title events.
			}
			if ev.SessionID != sess.ID || ev.Stats == nil {
				t.Errorf("got %+v, want the session's done event with stats", ev)
			}
			return
		case <-timeout:
			t.Fatal("no done event on the project topic")
		}
	}
}

// The UI holds one connection for every tab it has open, so one channel is
// registered under many topics. Unsubscribing it must not close the channel
// once per topic.
func TestHubSubscribeManyUsesOneChannel(t *testing.T) {
	h := NewHub()
	ch, unsub := h.SubscribeMany([]string{"a", "b", "b", ""})

	h.Publish("a", Event{SessionID: "a"})
	h.Publish("b", Event{SessionID: "b"})
	h.Publish("c", Event{SessionID: "c"}) // nobody is watching this one

	for _, want := range []string{"a", "b"} {
		select {
		case ev := <-ch:
			if ev.SessionID != want {
				t.Errorf("got %q, want %q", ev.SessionID, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("no event for topic %q", want)
		}
	}

	unsub()
	unsub() // idempotent: a stream torn down twice must not panic
	if _, open := <-ch; open {
		t.Error("channel still carries events after unsubscribing")
	}
}

// A subscriber dropped for falling behind on one topic is gone from all of
// them, and closed exactly once.
func TestHubDropsSlowSubscriberFromEveryTopic(t *testing.T) {
	h := NewHub()
	ch, unsub := h.SubscribeMany([]string{"a", "b"})
	defer unsub()
	for i := 0; i < subscriberBuffer+10; i++ {
		h.Publish("a", Event{SessionID: "a"})
	}
	h.Publish("b", Event{SessionID: "b"}) // must not panic on a closed channel
	drained := 0
	for range ch {
		drained++
	}
	if drained > subscriberBuffer {
		t.Errorf("drained %d, want at most %d", drained, subscriberBuffer)
	}
}

// busyLog records what OnBusy was told, in order. The listener runs under the
// runner's lock, so it does no more than append.
type busyLog struct {
	mu   sync.Mutex
	seen []bool
}

func (b *busyLog) record(busy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen = append(b.seen, busy)
}

func (b *busyLog) snapshot() []bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]bool(nil), b.seen...)
}

func TestOnBusyMarksTheEdgesOfWork(t *testing.T) {
	fp := &fakeProvider{
		script: []agent.Event{{Type: agent.EventDone, Usage: &agent.Usage{}}},
		gate:   make(chan struct{}),
	}
	r, _, sess := setup(t, fp)
	log := &busyLog{}
	r.OnBusy(log.record)

	if r.Busy() {
		t.Fatal("a runner with nothing in flight reports busy")
	}
	if _, err := r.Send(sess.ID, "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool { return len(log.snapshot()) == 1 }, "work to start")
	if !r.Busy() {
		t.Fatal("a runner with a turn in flight reports idle")
	}

	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn to finish")
	waitFor(t, func() bool { return len(log.snapshot()) == 2 }, "work to stop")

	if got := log.snapshot(); got[0] != true || got[1] != false {
		t.Fatalf("OnBusy saw %v, want [true false]", got)
	}
	if r.Busy() {
		t.Fatal("a finished turn left the runner busy")
	}
}

// A second turn starting while the first still runs is not a new spell of
// work, and neither is the first of the two finishing.
func TestOnBusySpansOverlappingTurns(t *testing.T) {
	fp := &fakeProvider{
		script: []agent.Event{{Type: agent.EventDone, Usage: &agent.Usage{}}},
		gate:   make(chan struct{}),
	}
	r, st, first := setup(t, fp)
	second := &store.Session{ID: "sess-2", ProjectID: first.ProjectID, Provider: "fake",
		Model: "m1", Effort: "high", Permission: "workspace"}
	if err := st.CreateSession(second); err != nil {
		t.Fatalf("create session: %v", err)
	}

	log := &busyLog{}
	r.OnBusy(log.record)
	for _, id := range []string{first.ID, second.ID} {
		if _, err := r.Send(id, "hi"); err != nil {
			t.Fatalf("send %s: %v", id, err)
		}
	}
	waitFor(t, func() bool { return len(log.snapshot()) == 1 }, "work to start")

	close(fp.gate)
	waitFor(t, func() bool { return !r.Running(first.ID) && !r.Running(second.ID) }, "turns to finish")
	waitFor(t, func() bool { return len(log.snapshot()) == 2 }, "work to stop")

	if got := log.snapshot(); len(got) != 2 || got[0] != true || got[1] != false {
		t.Fatalf("two overlapping turns reported %v, want one [true false]", got)
	}
}

// A turn the provider refuses to start is still a turn the runner took in and
// let go, so the tray has to end up at rest.
func TestOnBusyClearsWhenTheProviderRefuses(t *testing.T) {
	fp := &fakeProvider{runErr: errors.New("no such command")}
	r, _, sess := setup(t, fp)
	log := &busyLog{}
	r.OnBusy(log.record)

	if _, err := r.Send(sess.ID, "hi"); err == nil {
		t.Fatal("send with a failing provider succeeded, want an error")
	}
	if got := log.snapshot(); len(got) == 0 || got[len(got)-1] != false {
		t.Fatalf("OnBusy ended on %v, want it to finish reporting idle", got)
	}
	if r.Busy() {
		t.Fatal("a failed send left the runner busy")
	}
}

func TestShutdownWaitsForTurnsAndRejectsNewWork(t *testing.T) {
	r, _, sess := setup(t, &fakeProvider{gate: make(chan struct{})})
	if _, err := r.Send(sess.ID, "keep running"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { r.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the provider")
	}
	if r.Running(sess.ID) {
		t.Fatal("shutdown returned with an active turn")
	}
	if _, err := r.Send(sess.ID, "new work"); err == nil {
		t.Fatal("accepted work after shutdown")
	}
	ran := false
	r.background(func() { ran = true })
	r.Shutdown()
	if ran {
		t.Fatal("accepted background work after shutdown")
	}
}
