package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestProjectQueuePauseAPI(t *testing.T) {
	s, st := newTestServer(t)
	p, err := st.CreateProject("alpha", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d", p.ID)
	for _, paused := range []bool{true, false} {
		project := decode[store.Project](t, do(t, s, "PATCH", path, map[string]any{"queue_paused": paused}))
		if project.QueuePaused != paused {
			t.Fatalf("paused = %v, want %v", project.QueuePaused, paused)
		}
		projects := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
		if len(projects) != 1 || projects[0].QueuePaused != paused {
			t.Fatalf("projects = %+v", projects)
		}
	}
	resp := do(t, s, "PATCH", "/api/projects/999999", map[string]any{"queue_paused": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing project = %d", resp.StatusCode)
	}
}

func TestDeleteProjectDoesNotReleaseHeldQueue(t *testing.T) {
	_, st := newTestServer(t)
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	t.Cleanup(r.Shutdown)
	s := New(st, reg, r)
	t.Cleanup(s.remotes.stop)
	p, err := st.CreateProject("held", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("task-%02d", i)
		ids = append(ids, id)
		if err := st.CreateSession(&store.Session{ID: id, ProjectID: p.ID, Provider: "fake", Model: "fake-quick"}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			_, err = st.RequeueMessage(store.QueuedMessage{SessionID: id, Prompt: "retry",
				Provider: "fake", Model: "fake-quick"}, time.Now().Add(time.Hour).UnixMilli(), "model is at capacity")
		} else {
			_, err = st.EnqueueMessage(id, "@wait 60000", "fake", 0, "fake-quick", "")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	// Remove the held task first, while the other tasks still have work.
	if err := st.ReorderSessions(p.ID, ids); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := r.Hub().Subscribe(runner.ProjectTopic(p.ID))
	defer unsubscribe()
	resp := do(t, s, "DELETE", fmt.Sprintf("/api/projects/%d", p.ID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete project = %d", resp.StatusCode)
	}
	r.Shutdown()
	for len(events) > 0 {
		if event := <-events; event.Event.Type == "started" {
			t.Fatalf("project deletion started queued task %s", event.SessionID)
		}
	}
}
