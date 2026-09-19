package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func TestProfilesRenameAndPersist(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	resp := do(t, s, "PATCH", "/api/profiles/"+work.ID, map[string]string{"name": "  Office  "})
	if resp.StatusCode != 200 {
		t.Fatalf("rename: %d", resp.StatusCode)
	}
	renamed := decode[Profile](t, resp)
	if renamed.ID != work.ID || renamed.Name != "Office" {
		t.Fatalf("renamed profile: %+v", renamed)
	}
	other := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Personal"}))
	for _, name := range []string{"", "  ", "personal", strings.Repeat("x", 81)} {
		if resp := do(t, s, "PATCH", "/api/profiles/"+work.ID, map[string]string{"name": name}); resp.StatusCode < 400 {
			t.Fatalf("accepted rename %q", name)
		}
	}
	if resp := do(t, s, "PATCH", "/api/profiles/missing", map[string]string{"name": "Missing"}); resp.StatusCode != 404 {
		t.Fatalf("rename missing: %d", resp.StatusCode)
	}
	resp = do(t, s, "PATCH", "/api/profiles/default", map[string]string{"name": "Personal Default"})
	if resp.StatusCode != 200 {
		t.Fatalf("rename Default: %d", resp.StatusCode)
	}
	s.CloseProfiles()
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	listing := decode[struct {
		Profiles []Profile `json:"profiles"`
	}](t, do(t, s, "GET", "/api/profiles", nil))
	got := listing.Profiles
	byID := make(map[string]Profile, len(got))
	for _, p := range got {
		byID[p.ID] = p
	}
	if byID[work.ID].Name != "Office" || byID[other.ID].Name != "Personal" || byID["default"].Name != "Personal Default" {
		t.Fatalf("renames did not persist: %+v", got)
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

func TestProfilesShareSubscriptionsAndPersistSeparateFavourites(t *testing.T) {
	s := profileServer(t)
	p := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	homes := make(map[string]string)
	for _, id := range []string{"default", p.ID} {
		query := "?profile=" + id
		resp := do(t, s, "POST", "/api/providers/fake/accounts"+query, map[string]any{"alias": id, "default": true})
		if resp.StatusCode != 201 {
			t.Fatalf("create subscription: %d", resp.StatusCode)
		}
		account := decode[accountInfo](t, resp)
		homes[id] = account.Home
		resp = do(t, s, "POST", "/api/stars"+query, map[string]any{"provider": "fake", "account_id": account.ID, "model": id})
		if resp.StatusCode != 204 {
			t.Fatalf("add favourite: %d", resp.StatusCode)
		}
	}
	if homes["default"] == homes[p.ID] {
		t.Fatal("subscription directories overlap")
	}
	s.CloseProfiles()
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"default", p.ID} {
		query := "?profile=" + id
		stars := decode[[]store.Star](t, do(t, s, "GET", "/api/stars"+query, nil))
		if len(stars) != 1 || stars[0].Model != id {
			t.Fatalf("profile %s favourites: %+v", id, stars)
		}
		providers := decode[[]providerInfo](t, do(t, s, "GET", "/api/providers"+query, nil))
		if len(providers) != 1 || len(providers[0].Accounts) != 3 {
			t.Fatalf("profile %s subscriptions: %+v", id, providers)
		}
		account := providers[0].Accounts[2]
		if account.Alias != p.ID || !account.IsDefault || account.Home != homes[p.ID] {
			t.Fatalf("profile %s subscription changed: %+v", id, account)
		}
	}
}

func TestMoveProjectBetweenProfiles(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	other := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Other"}))
	project := decode[store.Project](t, do(t, s, "POST", "/api/projects", map[string]string{"path": t.TempDir()}))
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions", map[string]any{"project_id": project.ID, "provider": "fake", "model": "fake-quick", "title": "Moved task"}))
	account := decode[accountInfo](t, do(t, s, "POST", "/api/providers/fake/accounts", map[string]string{"alias": "Shared login"}))
	if err := s.store.SetSessionModel(session.ID, "fake", account.ID, "fake-quick", "", false); err != nil {
		t.Fatal(err)
	}
	source := "default"
	for _, destination := range []string{work.ID, other.ID, "default"} {
		url := fmt.Sprintf("/api/profiles/%s/projects/%d/move?profile=%s", destination, project.ID, source)
		response := do(t, s, "POST", url, nil)
		if response.StatusCode != 200 {
			t.Fatalf("move: %d", response.StatusCode)
		}
		result := decode[struct {
			ID int64 `json:"id"`
		}](t, response)
		if resp := do(t, s, "GET", fmt.Sprintf("/api/projects/%d?profile=%s", project.ID, source), nil); resp.StatusCode != 404 {
			t.Fatalf("source project: %d", resp.StatusCode)
		}
		moved := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/"+session.ID+"?profile="+destination, nil)).Session
		if moved.Title != "Moved task" || moved.ProjectID != result.ID || moved.AccountID != account.ID {
			t.Fatalf("moved task: %+v", moved)
		}
		project.ID, source = result.ID, destination
	}
	for _, tc := range []struct {
		destination, source, id string
		status                  int
	}{
		{"default", "default", fmt.Sprint(project.ID), 400},
		{"missing", "default", fmt.Sprint(project.ID), 404},
		{work.ID, "missing", fmt.Sprint(project.ID), 404},
		{work.ID, "default", "9999", 404},
		{work.ID, "default", "invalid", 400},
	} {
		response := do(t, s, "POST", fmt.Sprintf("/api/profiles/%s/projects/%s/move?profile=%s", tc.destination, tc.id, tc.source), nil)
		if response.StatusCode != tc.status {
			t.Fatalf("%+v: %d", tc, response.StatusCode)
		}
	}
	if err := s.store.SetSessionStatus(session.ID, "running"); err != nil {
		t.Fatal(err)
	}
	response := do(t, s, "POST", fmt.Sprintf("/api/profiles/%s/projects/%d/move", work.ID, project.ID), nil)
	if response.StatusCode != 409 {
		t.Fatalf("running project: %d", response.StatusCode)
	}
}

func TestProfilesReorder(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	check := func() {
		t.Helper()
		catalog := decode[struct {
			Profiles []Profile `json:"profiles"`
		}](t, do(t, s, "GET", "/api/profiles", nil))
		if len(catalog.Profiles) != 2 || catalog.Profiles[0] != work || catalog.Profiles[1].ID != "default" {
			t.Fatalf("unexpected order: %+v", catalog.Profiles)
		}
	}
	if resp := do(t, s, "PUT", "/api/profiles/order?profile="+work.ID, map[string]any{"ids": []string{work.ID, "default"}}); resp.StatusCode != 204 {
		t.Fatalf("reorder: %d", resp.StatusCode)
	}
	check()
	for _, ids := range [][]string{nil, {"default"}, {"default", "default"}, {"default", "missing"}, {"default", work.ID, "extra"}} {
		if resp := do(t, s, "PUT", "/api/profiles/order", map[string]any{"ids": ids}); resp.StatusCode != 400 {
			t.Fatalf("invalid order %v: %d", ids, resp.StatusCode)
		}
		check()
	}
	s.CloseProfiles()
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	check()
	// Failed persistence must leave the in-memory order untouched too.
	if err := os.Remove(s.profiles.path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.profiles.path(), 0700); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, s, "PUT", "/api/profiles/order", map[string]any{"ids": []string{"default", work.ID}}); resp.StatusCode != 500 {
		t.Fatalf("failed save: %d", resp.StatusCode)
	}
	check()
}

func TestProfilesShareSystemNamesButKeepModelVisibility(t *testing.T) {
	s := profileServer(t)
	p := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	query := "?profile=" + p.ID
	resp := do(t, s, "PATCH", "/api/providers/fake/accounts/0"+query, map[string]string{"alias": "Personal"})
	if resp.StatusCode != 200 {
		t.Fatalf("rename system: %d", resp.StatusCode)
	}
	for _, q := range []string{"", query} {
		providers := decode[[]providerInfo](t, do(t, s, "GET", "/api/providers"+q, nil))
		if a := providers[0].Accounts[0]; a.Alias != "Personal" || !a.System || a.ID != 0 {
			t.Fatalf("system: %+v", a)
		}
	}
	if resp := do(t, s, "PATCH", "/api/providers/fake/accounts/0", map[string]string{"alias": "", "home": t.TempDir()}); resp.StatusCode != 400 {
		t.Fatalf("invalid rename: %d", resp.StatusCode)
	}
	if err := s.profiles.running[p.ID].server.store.SetModelChoiceHidden("fake", 0, "fake-quick", true); err != nil {
		t.Fatal(err)
	}
	choices, err := s.store.ListHiddenModelChoices()
	if err != nil || len(choices) != 0 {
		t.Fatalf("visibility crossed profiles: %+v %v", choices, err)
	}
	s.CloseProfiles()
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	if name := s.profiles.running[p.ID].server.store.SystemAccountName("fake"); name != "Personal" {
		t.Fatal(name)
	}
}
