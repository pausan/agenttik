package runner

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/store"
)

// nextOfType reads a topic until an event of the given type arrives.
func nextOfType(t *testing.T, ch <-chan Event, typ agent.EventType) Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("stream closed waiting for %s", typ)
			}
			if ev.Event.Type == typ {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %s event", typ)
		}
	}
}

func TestApprovalReachesEveryWindowAndFirstAnswerWins(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	// Two windows: one with the task open, one only watching its project.
	tab, closeTab := r.Hub().Subscribe(sess.ID)
	defer closeTab()
	sidebar, closeSidebar := r.Hub().Subscribe(ProjectTopic(sess.ProjectID))
	defer closeSidebar()

	if _, err := r.Send(sess.ID, "@approve Bash {\"command\":\"ls\"}"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, tab, agent.EventApproval)
	nextOfType(t, sidebar, agent.EventApproval)
	if asked.Event.Approval.Tool != "Bash" || asked.Event.Approval.Input != `{"command":"ls"}` {
		t.Fatalf("approval = %+v", asked.Event.Approval)
	}
	// A window that connects now still finds it.
	pending := r.Approvals()
	if len(pending) != 1 || pending[0].SessionID != sess.ID || pending[0].Approval.ID != asked.Event.Approval.ID {
		t.Fatalf("pending = %+v", pending)
	}

	// Both windows click at once: exactly one answer is taken.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, allow := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = r.Answer(sess.ID, asked.Event.Approval.ID, agent.Reply{Allow: allow})
		}()
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("answers = %v; want exactly one accepted", errs)
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrApprovalGone) {
			t.Fatalf("losing answer = %v", err)
		}
	}
	resolved := nextOfType(t, sidebar, agent.EventApprovalResolved)
	if resolved.Event.Approval.Allowed == nil {
		t.Fatal("resolved event should say what was decided")
	}
	want := "denied"
	if *resolved.Event.Approval.Allowed {
		want = "allowed"
	}
	nextOfType(t, tab, agent.EventDone)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn should end")
	msgs, _ := st.ListMessages(sess.ID)
	if last := msgs[len(msgs)-1]; last.Role != store.RoleAssistant || strings.TrimSpace(last.Content) != want {
		t.Fatalf("reply = %+v, want %q", last, want)
	}
	if len(r.Approvals()) != 0 {
		t.Fatal("answered approval still pending")
	}
}

func TestStoppedTurnWithdrawsItsApprovals(t *testing.T) {
	r, _, sess := setup(t, fake.New())
	sidebar, unsubscribe := r.Hub().Subscribe(ProjectTopic(sess.ProjectID))
	defer unsubscribe()

	if _, err := r.Send(sess.ID, "@approve Bash rm -rf build"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, sidebar, agent.EventApproval)
	if err := r.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	resolved := nextOfType(t, sidebar, agent.EventApprovalResolved)
	if resolved.Event.Approval.ID != asked.Event.Approval.ID || resolved.Event.Approval.Allowed != nil {
		t.Fatalf("resolved = %+v", resolved.Event.Approval)
	}
	if len(r.Approvals()) != 0 {
		t.Fatal("stopped turn left an approval pending")
	}
	if err := r.Answer(sess.ID, asked.Event.Approval.ID, agent.Reply{Allow: true}); !errors.Is(err, ErrApprovalGone) {
		t.Fatalf("late answer = %v", err)
	}
}

func TestAnswerChecksTheSession(t *testing.T) {
	r, _, sess := setup(t, fake.New())
	tab, unsubscribe := r.Hub().Subscribe(sess.ID)
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, tab, agent.EventApproval)
	if err := r.Answer("other-session", asked.Event.Approval.ID, agent.Reply{Allow: true}); !errors.Is(err, ErrApprovalGone) {
		t.Fatalf("answer for another session = %v", err)
	}
	if len(r.Approvals()) != 1 {
		t.Fatal("a wrong-session answer must leave the approval pending")
	}
	r.Stop(sess.ID)
}

func TestQuestionNeedsACompleteAnswer(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	tab, unsubscribe := r.Hub().Subscribe(sess.ID)
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "@ask Which color? | Red, Blue"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, tab, agent.EventApproval).Event.Approval
	if len(asked.Questions) != 1 || len(asked.Questions[0].Options) != 2 {
		t.Fatalf("questions = %+v", asked.Questions)
	}
	// An empty answer is refused and leaves the question waiting.
	if err := r.Answer(sess.ID, asked.ID, agent.Reply{Allow: true}); !errors.Is(err, ErrBadReply) {
		t.Fatalf("empty answer = %v", err)
	}
	if err := r.Answer(sess.ID, asked.ID, agent.Reply{Allow: true,
		Answers: map[string][]string{"q": {"Green, please"}}}); err != nil {
		t.Fatal(err)
	}
	nextOfType(t, tab, agent.EventDone)
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "turn should end")
	msgs, _ := st.ListMessages(sess.ID)
	if got := strings.TrimSpace(msgs[len(msgs)-1].Content); got != "answer=Green, please" {
		t.Fatalf("reply = %q", got)
	}
}

func setApprovalMode(t *testing.T, st *store.Store, mode string, seconds int) {
	t.Helper()
	if err := st.SetGeneralConfig(store.GeneralConfig{NewItemPosition: "top",
		ApprovalMode: mode, ApprovalTimeout: seconds}); err != nil {
		t.Fatal(err)
	}
}

func lastReply(t *testing.T, r *Runner, st *store.Store, sessionID string) string {
	t.Helper()
	waitFor(t, func() bool { return !r.Running(sessionID) }, "turn should end")
	msgs, _ := st.ListMessages(sessionID)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == store.RoleAssistant {
			return strings.TrimSpace(msgs[i].Content)
		}
	}
	t.Fatal("no assistant reply")
	return ""
}

func TestQuestionAnswerHistory(t *testing.T) {
	for _, mode := range []string{store.ApprovalWait, store.ApprovalTimeout, store.ApprovalImmediate} {
		t.Run(mode, func(t *testing.T) {
			r, st, sess := setup(t, fake.New())
			setApprovalMode(t, st, mode, 1)
			tab, unsubscribe := r.Hub().Subscribe(sess.ID)
			defer unsubscribe()
			if _, err := r.Send(sess.ID, "@ask Language? | English, Spanish"); err != nil {
				t.Fatal(err)
			}
			want, role := "English", store.RoleAutomaticAnswer
			if mode == store.ApprovalWait {
				a := nextOfType(t, tab, agent.EventApproval).Event.Approval
				want, role = "Spanish", store.RoleAnswer
				if err := r.Answer(sess.ID, a.ID, agent.Reply{Allow: true, Answers: map[string][]string{"q": {want}}}); err != nil {
					t.Fatal(err)
				}
			}
			resolved := nextOfType(t, tab, agent.EventApprovalResolved)
			if resolved.Message == nil || resolved.Message.Role != role || resolved.Message.Content != "Language?\n"+want {
				t.Fatalf("resolution = %+v", resolved)
			}
			stored, err := st.GetMessage(resolved.Message.ID)
			if err != nil || *stored != *resolved.Message {
				t.Fatalf("stored = %+v, %v", stored, err)
			}
			lastReply(t, r, st, sess.ID)
		})
	}
}

func TestQuestionHistoryHidesSecretsAndRecordsSkips(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	p := &PendingApproval{SessionID: sess.ID, ProjectID: sess.ProjectID, Approval: &agent.Approval{ID: "secret", Questions: []agent.Question{{ID: "password", Question: "Password?", Secret: true}}}}
	r.publishReply(p, agent.Reply{Allow: true, Answers: map[string][]string{"password": {"very-secret"}}}, false)
	r.publishReply(p, agent.Reply{}, true)
	r.publishResolved(p, nil, false)
	msgs, err := st.ListMessages(sess.ID)
	if err != nil || len(msgs) != 3 || msgs[0].Content != "Password?\n[hidden]" || msgs[1].Content != "Password?\nSkipped" || msgs[2].Role != store.RoleQuestion || msgs[2].Content != "Password?\nClosed without an accepted answer" {
		t.Fatalf("messages = %+v, %v", msgs, err)
	}
}

func TestTimedOutApprovalIsAllowedForTheUser(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	setApprovalMode(t, st, store.ApprovalTimeout, 1)
	sidebar, unsubscribe := r.Hub().Subscribe(ProjectTopic(sess.ProjectID))
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, sidebar, agent.EventApproval).Event.Approval
	if asked.ExpiresIn != 1000 {
		t.Fatalf("expires in %d ms, want 1000", asked.ExpiresIn)
	}
	if left := r.Approvals()[0].Approval.ExpiresIn; left <= 0 || left > 1000 {
		t.Fatalf("listed with %d ms left", left)
	}
	resolved := nextOfType(t, sidebar, agent.EventApprovalResolved).Event.Approval
	if !resolved.Auto || resolved.Allowed == nil || !*resolved.Allowed {
		t.Fatalf("resolved = %+v", resolved)
	}
	if got := lastReply(t, r, st, sess.ID); got != "allowed" {
		t.Fatalf("reply = %q", got)
	}
}

func TestTimedOutQuestionPicksTheRecommendedOption(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	setApprovalMode(t, st, store.ApprovalTimeout, 1)
	if _, err := r.Send(sess.ID, "@ask Which store? | Redis, SQLite (Recommended)"); err != nil {
		t.Fatal(err)
	}
	if got := lastReply(t, r, st, sess.ID); got != "answer=SQLite (Recommended)" {
		t.Fatalf("reply = %q", got)
	}
}

func TestImmediateModeAsksNobody(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	setApprovalMode(t, st, store.ApprovalImmediate, 30)
	tab, unsubscribe := r.Hub().Subscribe(sess.ID)
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	for ev := range tab {
		if ev.Event.Type == agent.EventApproval {
			t.Fatal("an immediate answer should not be shown")
		}
		if ev.Event.Type == agent.EventDone {
			break
		}
	}
	if got := lastReply(t, r, st, sess.ID); got != "allowed" {
		t.Fatalf("reply = %q", got)
	}
}

func TestWaitModeAndHoldKeepTheRequest(t *testing.T) {
	r, st, sess := setup(t, fake.New())
	setApprovalMode(t, st, store.ApprovalWait, 1)
	tab, unsubscribe := r.Hub().Subscribe(sess.ID)
	defer unsubscribe()
	if _, err := r.Send(sess.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	if asked := nextOfType(t, tab, agent.EventApproval).Event.Approval; asked.ExpiresIn != 0 {
		t.Fatalf("wait mode counts down: %d", asked.ExpiresIn)
	}

	setApprovalMode(t, st, store.ApprovalTimeout, 1)
	// A second task, asked under the timeout, is held before it runs out.
	other := &store.Session{ID: "sess-2", ProjectID: sess.ProjectID, Provider: "fake", Model: "m1", Permission: "workspace"}
	if err := st.CreateSession(other); err != nil {
		t.Fatal(err)
	}
	otherTab, unsubscribeOther := r.Hub().Subscribe(other.ID)
	defer unsubscribeOther()
	if _, err := r.Send(other.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	asked := nextOfType(t, otherTab, agent.EventApproval).Event.Approval
	if err := r.Hold(other.ID, asked.ID); err != nil {
		t.Fatal(err)
	}
	if held := nextOfType(t, otherTab, agent.EventApproval).Event.Approval; held.ID != asked.ID || held.ExpiresIn != 0 || !held.Held {
		t.Fatalf("held = %+v", held)
	}
	time.Sleep(1500 * time.Millisecond)
	if n := len(r.Approvals()); n != 2 {
		t.Fatalf("%d pending after the timeout passed, want both still waiting", n)
	}
	r.Stop(sess.ID)
	r.Stop(other.ID)
}
