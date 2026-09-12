package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/orchestrator"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestOrchestratorSettingsAndFiles(t *testing.T) {
	s, st := newTestServer(t)
	cfg := decode[store.OrchestratorConfig](t, do(t, s, "GET", "/api/orchestrator", nil))
	if cfg.Enabled || cfg.ProjectID != 0 {
		t.Fatalf("initial config = %+v", cfg)
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Fatal("reading settings created a folder")
	}
	events, unsub := s.runner.Hub().Subscribe(runner.ProjectsTopic)
	defer unsub()
	resp := do(t, s, "PUT", "/api/orchestrator", map[string]bool{"enabled": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable status = %d", resp.StatusCode)
	}
	cfg = decode[store.OrchestratorConfig](t, resp)
	if !cfg.Enabled || cfg.Prompt != orchestrator.DefaultPrompt {
		t.Fatalf("enabled config = %+v", cfg)
	}
	entries, err := os.ReadDir(cfg.Path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("initial folder = %v, %v", entries, err)
	}
	select {
	case event := <-events:
		if event.Event.Type != runner.EventProjectsChanged {
			t.Fatalf("enable event = %+v", event)
		}
	default:
		t.Fatal("enable did not notify other windows")
	}
	file := filepath.Join(cfg.Path, "notes.md")
	if err := os.WriteFile(file, []byte("keep these notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d", cfg.ProjectID)
	resp = do(t, s, "PATCH", path, map[string]any{"name": "Coordinator", "prompt": "My instructions", "archived": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive status = %d", resp.StatusCode)
	}
	cfg = decode[store.OrchestratorConfig](t, do(t, s, "GET", "/api/orchestrator", nil))
	if cfg.Enabled || cfg.Name != "Coordinator" || cfg.Prompt != "My instructions" {
		t.Fatalf("archived config = %+v", cfg)
	}
	resp = do(t, s, "DELETE", path, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	cfg = decode[store.OrchestratorConfig](t, do(t, s, "PUT", "/api/orchestrator", map[string]bool{"enabled": true}))
	if !cfg.Enabled || cfg.Name != "Coordinator" || cfg.Prompt != "My instructions" {
		t.Fatalf("reenabled config = %+v", cfg)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep these notes" {
		t.Fatalf("notes = %q, %v", data, err)
	}
	cfg = decode[store.OrchestratorConfig](t, do(t, s, "POST", "/api/orchestrator/prompt/reset", nil))
	project, _ := st.GetProject(cfg.ProjectID)
	if cfg.Prompt != orchestrator.DefaultPrompt || project.Prompt != cfg.Prompt {
		t.Fatal("reset did not restore current build's default")
	}
}

func TestOrchestratorEnableFailureAndBusyDisable(t *testing.T) {
	s, st := newTestServer(t)
	cfg, _ := st.GetOrchestratorConfig()
	if err := os.WriteFile(cfg.Path, []byte("a file occupies the folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := do(t, s, "PUT", "/api/orchestrator", map[string]bool{"enabled": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("file collision status = %d", resp.StatusCode)
	}
	cfg, _ = st.GetOrchestratorConfig()
	if cfg.Enabled || cfg.ProjectID != 0 {
		t.Fatal("failed enable left a project")
	}
	if err := os.Remove(cfg.Path); err != nil {
		t.Fatal(err)
	}
	cfg = decode[store.OrchestratorConfig](t, do(t, s, "PUT", "/api/orchestrator", map[string]bool{"enabled": true}))
	if err := st.CreateSession(&store.Session{ID: "busy", ProjectID: cfg.ProjectID, Provider: "fake", Model: "m", Status: store.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{"PUT", "/api/orchestrator", map[string]bool{"enabled": false}},
		{"PATCH", fmt.Sprintf("/api/projects/%d", cfg.ProjectID), map[string]bool{"archived": true}},
	} {
		resp := do(t, s, request.method, request.path, request.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s = %d", request.method, request.path, resp.StatusCode)
		}
	}
	cfg, _ = st.GetOrchestratorConfig()
	if !cfg.Enabled {
		t.Fatal("busy project was hidden")
	}
}

func TestControlAPICrossProjectTaskLifecycle(t *testing.T) {
	_, st := newTestServer(t)
	reg := agent.NewRegistry(fake.New())
	r := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, r)
	t.Cleanup(r.StopAll)
	project, _ := st.CreateProject("Target", t.TempDir())
	events, unsub := r.Hub().Subscribe(runner.ProjectsTopic)
	defer unsub()
	task := decode[store.Session](t, do(t, s, "POST", "/api/sessions", map[string]any{
		"project_id": project.ID, "provider": "fake", "title": "Cross-project task",
	}))
	path := "/api/sessions/" + task.ID
	resp := do(t, s, "POST", path+"/messages", map[string]string{"prompt": "@wait 60000\nanswer"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("send status = %d", resp.StatusCode)
	}
	detail := decode[sessionDetail](t, do(t, s, "GET", path, nil))
	if !detail.Running || detail.Session.ProjectID != project.ID || detail.Session.Status != store.StatusRunning {
		t.Fatalf("running detail = %+v", detail)
	}
	projects := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	if len(projects) != 1 || len(projects[0].RecentSessions) != 1 || projects[0].RecentSessions[0].Status != store.StatusRunning {
		t.Fatalf("live projects = %+v", projects)
	}
	resp = do(t, s, "POST", path+"/stop", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("stop status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	for r.Running(task.ID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	resp = do(t, s, "PATCH", path, map[string]bool{"done": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive status = %d", resp.StatusCode)
	}
	archived := decode[store.Session](t, resp)
	if archived.DoneAt == 0 {
		t.Fatal("task not archived")
	}
	resp = do(t, s, "PATCH", fmt.Sprintf("/api/projects/%d", project.ID), map[string]bool{"archived": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive project status = %d", resp.StatusCode)
	}
	archivedProjects := decode[[]store.Project](t, do(t, s, "GET", "/api/projects?archived=true", nil))
	if len(archivedProjects) != 1 || archivedProjects[0].ID != project.ID {
		t.Fatalf("archived projects = %+v", archivedProjects)
	}
	// Creation and archive are published even when no browser caused them.
	changes := 0
	for len(events) > 0 {
		event := <-events
		if event.Event.Type == runner.EventSessionChanged && event.SessionID == task.ID {
			changes++
		}
	}
	if changes != 2 {
		t.Fatalf("task change events = %d, want 2", changes)
	}
}
