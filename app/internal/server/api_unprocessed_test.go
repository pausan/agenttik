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

// A prompt the agent never processed is sent or queued from the transcript,
// and moves rather than being written again; a prompt that ran is refused.
func TestUnprocessedPromptRoutes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, r)
	t.Cleanup(s.remotes.stop)
	settled := func() {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if queued, _ := st.ListQueuedMessages("owner"); !r.Running("owner") && len(queued) == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("task still busy")
			}
		}
	}

	p, _ := st.CreateProject("alpha", t.TempDir())
	if err := st.CreateSession(&store.Session{ID: "owner", ProjectID: p.ID, Provider: "fake",
		Model: "fake-quick", Permission: "workspace"}); err != nil {
		t.Fatal(err)
	}
	// What Stop leaves of two queued prompts.
	b, _ := st.AddMessage("owner", 0, store.RoleUser, "Is game B next?")
	c, _ := st.AddMessage("owner", 0, store.RoleUser, "And game C?")
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/sessions/owner/messages/nope/send", http.StatusBadRequest},
		{"/api/sessions/missing/messages/" + itoa(b.ID) + "/send", http.StatusNotFound},
		{"/api/sessions/owner/messages/99999/queue", http.StatusNotFound},
	} {
		resp := do(t, s, "POST", tc.path, nil)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("POST %s = %d, want %d", tc.path, resp.StatusCode, tc.want)
		}
	}

	resp := do(t, s, "POST", "/api/sessions/owner/messages/"+itoa(b.ID)+"/send", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("send: status %d", resp.StatusCode)
	}
	if sent := decode[struct {
		Turn   store.Turn `json:"turn"`
		Joined bool       `json:"joined"`
	}](t, resp); sent.Turn.ID == 0 || sent.Joined {
		t.Fatalf("send answered %+v", sent)
	}
	settled()
	if resp := do(t, s, "POST", "/api/sessions/owner/messages/"+itoa(c.ID)+"/queue", nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("queue: status %d", resp.StatusCode)
	}
	settled()

	detail := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/owner", nil))
	var prompts []store.Message
	for _, m := range detail.Messages {
		if m.TurnID == 0 {
			t.Fatalf("message without a turn: %+v", m)
		}
		if m.Role == store.RoleUser {
			prompts = append(prompts, m)
		}
	}
	if len(detail.Turns) != 2 || len(prompts) != 2 || prompts[0].Content != "Is game B next?" || prompts[1].Content != "And game C?" {
		t.Fatalf("turns %d, prompts %+v", len(detail.Turns), prompts)
	}
	resp = do(t, s, "POST", "/api/sessions/owner/messages/"+itoa(prompts[0].ID)+"/send", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("sending a prompt that ran: status %d", resp.StatusCode)
	}
}
