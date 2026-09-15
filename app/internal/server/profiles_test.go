package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/single"
	"github.com/pausan/agenttik/app/internal/store"
)

func profileServer(t *testing.T) *Server {
	t.Helper()
	s, _ := newTestServer(t)
	s.registry = agent.NewRegistry(fake.New())
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown() })
	return s
}

func TestProfilesIsolateProjectsTasksAndSettings(t *testing.T) {
	s := profileServer(t)
	p := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	if p.ID == "" {
		t.Fatal("missing profile")
	}
	path := t.TempDir()
	a := decode[store.Project](t, do(t, s, "POST", "/api/projects", map[string]string{"path": path}))
	b := decode[store.Project](t, do(t, s, "POST", "/api/projects?profile="+p.ID, map[string]string{"path": path}))
	if a.ID == 0 || b.ID == 0 {
		t.Fatal("same folder must be accepted in both profiles")
	}
	body := map[string]any{"project_id": b.ID, "provider": "fake", "model": "fake-quick", "title": "Work task"}
	resp := do(t, s, "POST", "/api/sessions?profile="+p.ID, body)
	if resp.StatusCode != 201 {
		t.Fatalf("create task: %d", resp.StatusCode)
	}
	sessions := decode[[]store.Session](t, do(t, s, "GET", "/api/sessions", nil))
	if len(sessions) != 0 {
		t.Fatalf("default leaked tasks: %+v", sessions)
	}
	sessions = decode[[]store.Session](t, do(t, s, "GET", "/api/sessions?profile="+p.ID, nil))
	if len(sessions) != 1 {
		t.Fatalf("work tasks: %+v", sessions)
	}
	resp = do(t, s, "PUT", "/api/general?profile="+p.ID, map[string]string{"new_item_position": "bottom"})
	if resp.StatusCode != 200 {
		t.Fatalf("settings: %d", resp.StatusCode)
	}
	config := decode[map[string]any](t, do(t, s, "GET", "/api/general", nil))
	if config["new_item_position"] == "bottom" {
		t.Fatal("settings crossed profiles")
	}
	// Requests carrying an unknown profile must never fall back to Default.
	if resp := do(t, s, "POST", "/api/projects?profile=missing", map[string]string{"path": t.TempDir()}); resp.StatusCode != 404 {
		t.Fatalf("unknown profile: %d", resp.StatusCode)
	}
	addr := single.Addr(filepath.Join(s.profiles.dir(p.ID), "agenttik.lock"))
	resp, err := http.Get("http://" + addr + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if len(decode[[]store.Session](t, resp)) != 1 {
		t.Fatal("CLI endpoint is not profile-local")
	}
}

func TestProfilesPersistAndDeleteOnlyTheirData(t *testing.T) {
	s := profileServer(t)
	p := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	dir := s.profiles.dir(p.ID)
	path := t.TempDir()
	do(t, s, "POST", "/api/projects?profile="+p.ID, map[string]string{"path": path})
	s.CloseProfiles()
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	projects := decode[[]store.Project](t, do(t, s, "GET", "/api/projects?profile="+p.ID, nil))
	if len(projects) != 1 {
		t.Fatalf("projects did not persist: %+v", projects)
	}
	if resp := do(t, s, "DELETE", "/api/profiles/"+p.ID, nil); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile directory remains: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("project files removed", err)
	}
	if _, err := os.Stat(s.store.Path()); err != nil {
		t.Fatal("default database removed", err)
	}
	if resp := do(t, s, "GET", "/api/projects?profile="+p.ID, nil); resp.StatusCode != 404 {
		t.Fatalf("deleted profile: %d", resp.StatusCode)
	}
}

func TestProfilesValidateNamesAndProtectDefault(t *testing.T) {
	s := profileServer(t)
	for _, name := range []string{"", "  ", "DEFAULT"} {
		resp := do(t, s, "POST", "/api/profiles", map[string]string{"name": name})
		if resp.StatusCode < 400 {
			t.Fatalf("accepted %q", name)
		}
	}
	if resp := do(t, s, "DELETE", "/api/profiles/default", nil); resp.StatusCode != 400 {
		t.Fatalf("delete Default: %d", resp.StatusCode)
	}
	if resp := do(t, s, "DELETE", "/api/profiles/missing", nil); resp.StatusCode != 404 {
		t.Fatalf("delete missing: %d", resp.StatusCode)
	}
}

func TestDeletingProfileStopsOnlyItsRunner(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	other := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Other"}))
	var otherSession string
	for _, p := range []Profile{work, other} {
		rt := s.profiles.running[p.ID]
		project, err := rt.server.store.CreateProject("project", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		session := &store.Session{ID: p.ID, ProjectID: project.ID, Provider: "fake", Model: "fake-quick", Title: "Wait"}
		if err := rt.server.store.CreateSession(session); err != nil {
			t.Fatal(err)
		}
		if _, err := rt.server.runner.Send(session.ID, "@wait 60000"); err != nil {
			t.Fatal(err)
		}
		otherSession = session.ID
	}
	deletedRunner := s.profiles.running[work.ID].server.runner
	if resp := do(t, s, "DELETE", "/api/profiles/"+work.ID, nil); resp.StatusCode != 204 {
		t.Fatalf("delete running profile: %d", resp.StatusCode)
	}
	if deletedRunner.Running(work.ID) {
		t.Fatal("deleted profile still running")
	}
	if !s.profiles.running[other.ID].server.runner.Running(otherSession) {
		t.Fatal("deletion stopped another profile")
	}
}
