package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestPinnedPromptValidationAndPersistence(t *testing.T) {
	s, st := newTestServer(t)
	s.registry = agent.NewRegistry(singleLoginProvider{name: "pin-test"})
	p, err := st.CreateProject("pins", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resp := do(t, s, "POST", "/api/schedules", map[string]any{
		"project_id": p.ID, "provider": "pin-test", "prompt": "Saved prompt", "every": "pinned",
		"remaining": -1, "interval_minutes": 15,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	pin := decode[scheduleDetail](t, resp).Schedule
	if pin.Every != store.EveryPinned || pin.NextRunAt != 0 || pin.Remaining != 0 || pin.IntervalMinutes != 0 {
		t.Fatalf("pin = %+v", pin)
	}
	path := fmt.Sprintf("/api/schedules/%d", pin.ID)
	for _, body := range []map[string]any{
		{"prompt": "  "}, {"every": "interval", "interval_minutes": 1}, {"paused": false}, {"remaining": -1},
	} {
		resp := do(t, s, "PATCH", path, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("patch %+v = %d", body, resp.StatusCode)
		}
	}
	resp = do(t, s, "PATCH", path, map[string]string{"prompt": "Edited prompt"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit = %d", resp.StatusCode)
	}
	resp = do(t, s, "POST", path+"/run", map[string]string{"prompt": "  "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty run = %d", resp.StatusCode)
	}
	if err := st.SetScheduleDone(pin.ID, true); err != nil {
		t.Fatal(err)
	}
	resp = do(t, s, "POST", path+"/run", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("archived run = %d", resp.StatusCode)
	}
	saved, err := st.GetSchedule(pin.ID)
	if err != nil || saved.Prompt != "Edited prompt" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	projects, err := st.ListProjects()
	if err != nil || len(projects[0].RecentSessions) != 0 {
		t.Fatalf("unexpected spawned tasks: %+v, %v", projects, err)
	}
}
