package server

import (
	"fmt"
	"net/http"
	"testing"

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
