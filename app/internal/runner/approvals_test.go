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
			errs[i] = r.Answer(sess.ID, asked.Event.Approval.ID, allow)
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
	if err := r.Answer(sess.ID, asked.Event.Approval.ID, true); !errors.Is(err, ErrApprovalGone) {
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
	if err := r.Answer("other-session", asked.Event.Approval.ID, true); !errors.Is(err, ErrApprovalGone) {
		t.Fatalf("answer for another session = %v", err)
	}
	if len(r.Approvals()) != 1 {
		t.Fatal("a wrong-session answer must leave the approval pending")
	}
	r.Stop(sess.ID)
}
