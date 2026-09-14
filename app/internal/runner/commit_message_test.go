package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestCommitMessageIsolated(t *testing.T) {
	sp := newSmallProvider("  Add greeting\n\nExplain the new greeting.\n")
	r, _, sess := setup(t, sp)
	diff := "diff --git a/hello.txt b/hello.txt\n+hello"
	got := r.CommitMessage(sess.Provider, sess.AccountID, diff)
	if got != "Add greeting\n\nExplain the new greeting." {
		t.Fatalf("message = %q", got)
	}
	req := <-sp.asked
	if !req.Isolated || req.Permission != agent.PermissionPlan || req.SessionID != "" || req.WorkDir != os.TempDir() || req.Model != "m1" || req.Effort != "low" {
		t.Fatalf("unsafe or non-lightweight request: %+v", req)
	}
	if !strings.Contains(req.Prompt, diff) {
		t.Fatal("missing staged diff")
	}
	if got := r.CommitMessage("missing", 0, diff); got != "" {
		t.Fatalf("unknown provider returned %q", got)
	}
}

type failedCommitProvider struct{ *smallProvider }

func (p *failedCommitProvider) Run(_ context.Context, _ agent.TurnRequest) (<-chan agent.Event, error) {
	events := make(chan agent.Event, 2)
	events <- agent.Event{Type: agent.EventText, Text: "Incomplete draft"}
	events <- agent.Event{Type: agent.EventError, Text: "Provider failed"}
	close(events)
	return events, nil
}
func TestCommitMessageDiscardsPartialFailure(t *testing.T) {
	r, _, sess := setup(t, &failedCommitProvider{newSmallProvider("")})
	if got := r.CommitMessage(sess.Provider, sess.AccountID, "diff"); got != "" {
		t.Fatalf("partial response: %q", got)
	}
}
func TestSmallRequestTimeout(t *testing.T) {
	sp := newSmallProvider("Late response")
	sp.gate = make(chan struct{})
	r, _, sess := setup(t, sp)
	if got := r.askSmall(sess.Provider, sess.AccountID, "diff", time.Millisecond); got != "" {
		t.Fatalf("late response: %q", got)
	}
}
