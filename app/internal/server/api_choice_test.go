package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

// A prompt meant for another model than the running turn's is refused with
// both choices, for the window to ask from, until the request says what to do.
func TestPromptOnAnotherChoice(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, r)
	t.Cleanup(s.remotes.stop)
	t.Cleanup(func() { r.Stop("owner") })
	waitFor := func(cond func() bool, what string) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}

	p, _ := st.CreateProject("alpha", t.TempDir())
	if err := st.CreateSession(&store.Session{ID: "owner", ProjectID: p.ID, Provider: "fake",
		Model: "fake-quick", Effort: "high", Permission: "workspace"}); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, s, "POST", "/api/sessions/owner/messages", map[string]any{
		"prompt": "@open\n@background start b1 build game A\nBuild started."}); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("send: status %d", resp.StatusCode)
	}
	waitFor(func() bool { return r.AcceptsPrompts("owner") }, "the turn to take prompts")
	if resp := do(t, s, "PATCH", "/api/sessions/owner", map[string]any{"model": "fake-careful", "effort": ""}); resp.StatusCode != http.StatusOK {
		t.Fatalf("model change: status %d", resp.StatusCode)
	}
	detail := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/owner", nil))
	if detail.RunningChoice == nil || detail.RunningChoice.Model != "fake-quick" || detail.Session.Model != "fake-careful" {
		t.Fatalf("running choice %+v, session model %q", detail.RunningChoice, detail.Session.Model)
	}

	resp := do(t, s, "POST", "/api/sessions/owner/messages", map[string]any{"prompt": "Status?"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("send on another model: status %d", resp.StatusCode)
	}
	refused := decode[choiceConflict](t, resp)
	if refused.Running.Model != "fake-quick" || refused.Running.Effort != "high" || refused.Wanted.Model != "fake-careful" ||
		!strings.Contains(refused.Error, "fake-quick") {
		t.Fatalf("refusal = %+v", refused)
	}
	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{"/api/sessions/owner/messages", map[string]any{"prompt": "Status?", "on_mismatch": "stop"}},
		{"/api/sessions/owner/messages", map[string]any{"prompt": "Status?", "on_mismatch": "maybe"}},
		{"/api/sessions/owner/queue/force", map[string]any{"queued_id": 1, "on_mismatch": "maybe"}},
	} {
		resp := do(t, s, "POST", tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST %s %v = %d, want 400", tc.path, tc.body, resp.StatusCode)
		}
	}

	resp = do(t, s, "POST", "/api/sessions/owner/messages", map[string]any{
		"prompt": "@background end b1 completed built\nAll done.", "on_mismatch": "join"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("send on the turn's model: status %d", resp.StatusCode)
	}
	waitFor(func() bool { return !r.Running("owner") }, "the turn to end")
	turns, _ := st.ListTurns("owner")
	if len(turns) != 1 || turns[0].Model != "fake-quick" {
		t.Fatalf("turns = %+v", turns)
	}
}
