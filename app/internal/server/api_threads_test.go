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

func TestThreadRoutes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, r)
	t.Cleanup(s.remotes.stop)
	idle := func(id string) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); r.Running(id); {
			if time.Now().After(deadline) {
				t.Fatalf("%s still running", id)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	p, _ := st.CreateProject("alpha", t.TempDir())
	sess := &store.Session{ID: "main", ProjectID: p.ID, Title: "Main", Provider: "fake", Model: "fake-quick", Permission: "workspace"}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Send(sess.ID, "first answer"); err != nil {
		t.Fatal(err)
	}
	idle(sess.ID)
	if _, err := r.Send(sess.ID, "second answer"); err != nil {
		t.Fatal(err)
	}
	idle(sess.ID)
	messages, _ := st.ListMessages(sess.ID)
	first, second := messages[1].ID, messages[3].ID
	other := &store.Session{ID: "other", ProjectID: p.ID, Provider: "fake"}
	st.CreateSession(other)

	for _, bad := range []map[string]any{
		{"message_id": first, "quote": "", "prompt": "why?"},
		{"message_id": first, "quote": "first", "prompt": " "},
		{"message_id": 99999, "quote": "first", "prompt": "why?"},
	} {
		if resp := do(t, s, "POST", "/api/sessions/other/threads", bad); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", bad, resp.StatusCode)
		}
	}

	created := decode[struct {
		Thread store.Session `json:"thread"`
	}](t, do(t, s, "POST", "/api/sessions/main/threads", map[string]any{"message_id": first, "quote": "first", "prompt": "Why first?\nmore"}))
	thread := created.Thread
	if thread.ParentID != "main" || thread.AnchorMessageID != first || thread.Title != "Why first?" || thread.Provider != "fake" {
		t.Fatalf("thread = %+v", thread)
	}
	idle(thread.ID)
	later := decode[struct {
		Thread store.Session `json:"thread"`
	}](t, do(t, s, "POST", "/api/sessions/main/threads", map[string]any{"message_id": second, "quote": "second", "prompt": "And this?"})).Thread
	idle(later.ID)

	// The conversation keeps its own transcript and lists its threads.
	detail := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/main", nil))
	if len(detail.Messages) != 4 || len(detail.Threads) != 2 || detail.Threads[0].ID != thread.ID ||
		detail.Threads[0].AnchorQuote != "first" || detail.Threads[0].Replies != 1 {
		t.Fatalf("detail: %d messages, threads %+v", len(detail.Messages), detail.Threads)
	}
	// Threads are not tasks.
	for _, listed := range decode[[]store.Session](t, do(t, s, "GET", "/api/sessions", nil)) {
		if listed.ParentID != "" {
			t.Fatalf("thread listed as a task: %+v", listed)
		}
	}
	if resp := do(t, s, "POST", "/api/sessions/"+thread.ID+"/threads", map[string]any{"message_id": first, "quote": "x", "prompt": "y"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("thread of a thread: status %d", resp.StatusCode)
	}

	// Spawning makes a task that shows the thread and points back at it.
	if resp := do(t, s, "POST", "/api/sessions/main/spawn", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("spawning a task: status %d", resp.StatusCode)
	}
	task := decode[store.Session](t, do(t, s, "POST", "/api/sessions/"+thread.ID+"/spawn", nil))
	if task.ParentID != "" || task.OriginID != thread.ID || task.Title != "Why first?" {
		t.Fatalf("task = %+v", task)
	}
	spawned := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/"+task.ID, nil))
	if len(spawned.Messages) != 2 || spawned.Messages[0].Content != "Why first?\nmore" || len(spawned.Turns) != 0 ||
		spawned.Origin == nil || spawned.Origin.ParentID != "main" || spawned.Origin.ParentTitle != "Main" || spawned.Origin.Quote != "first" {
		t.Fatalf("spawned: %+v origin %+v", spawned.Messages, spawned.Origin)
	}
	if again := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/main", nil)); len(again.Threads) != 2 {
		t.Fatalf("spawning moved the thread: %+v", again.Threads)
	}

	// Editing the second prompt removes the thread about its answer only.
	if resp := do(t, s, "POST", "/api/sessions/main/messages/"+itoa(messages[2].ID)+"/edit", map[string]any{"prompt": "redo", "enqueue": true}); resp.StatusCode >= 300 {
		t.Fatalf("edit: status %d", resp.StatusCode)
	}
	if left := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/main", nil)).Threads; len(left) != 1 || left[0].ID != thread.ID {
		t.Fatalf("after edit: %+v", left)
	}
	// Deleting the conversation deletes its threads; the spawned task stays.
	if resp := do(t, s, "DELETE", "/api/sessions/main", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	if _, err := st.GetSession(thread.ID); err != store.ErrNotFound {
		t.Fatalf("thread survived its conversation: %v", err)
	}
	if detail := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/"+task.ID, nil)); detail.Origin != nil {
		t.Fatalf("origin of a deleted thread: %+v", detail.Origin)
	}
}
