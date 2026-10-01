package server

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestApprovalRoutes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, r)

	p, _ := st.CreateProject("alpha", t.TempDir())
	sess := &store.Session{ID: "sess-1", ProjectID: p.ID, Provider: "fake", Model: "fake-quick", Permission: "workspace"}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Send(sess.ID, "@approve Bash ls"); err != nil {
		t.Fatal(err)
	}
	var pending []runner.PendingApproval
	for deadline := time.Now().Add(2 * time.Second); len(pending) == 0 && time.Now().Before(deadline); {
		pending = decode[[]runner.PendingApproval](t, do(t, s, "GET", "/api/approvals", nil))
		time.Sleep(10 * time.Millisecond)
	}
	if len(pending) != 1 || pending[0].Approval.Tool != "Bash" {
		t.Fatalf("pending = %+v", pending)
	}
	path := "/api/sessions/sess-1/approvals/" + pending[0].Approval.ID
	if resp := do(t, s, "POST", path, map[string]any{}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing allow: status %d, want 400", resp.StatusCode)
	}
	if resp := do(t, s, "POST", path, map[string]bool{"allow": true}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("answer: status %d", resp.StatusCode)
	}
	// Another window clicking after it was answered is told so.
	if resp := do(t, s, "POST", path, map[string]bool{"allow": false}); resp.StatusCode != http.StatusConflict {
		t.Errorf("second answer: status %d, want 409", resp.StatusCode)
	}
	if left := decode[[]runner.PendingApproval](t, do(t, s, "GET", "/api/approvals", nil)); len(left) != 0 {
		t.Errorf("still pending: %+v", left)
	}
}
