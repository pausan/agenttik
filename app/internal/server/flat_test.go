package server

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/orchestrator"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/app/internal/terminals"
)

type flatProject struct {
	store.Project
	Source struct {
		Machine string `json:"machine"`
		Profile string `json:"profile"`
		Remote  bool   `json:"remote"`
	} `json:"source"`
}

func flatProjects(t *testing.T, s *Server) []flatProject {
	t.Helper()
	resp := do(t, s, "GET", "/api/projects?profile=all", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("flat projects: %d", resp.StatusCode)
	}
	return decode[[]flatProject](t, resp)
}

func flatNamed(t *testing.T, list []flatProject, name, profile string) flatProject {
	t.Helper()
	for _, p := range list {
		if p.Name == name && p.Source.Profile == profile {
			return p
		}
	}
	t.Fatalf("no %s in %s: %+v", name, profile, list)
	return flatProject{}
}

func TestFlatViewListsAndRoutesEveryLocalProfile(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	folder := t.TempDir()
	home := decode[store.Project](t, do(t, s, "POST", "/api/projects", map[string]string{"path": folder, "name": "Same"}))
	away := decode[store.Project](t, do(t, s, "POST", "/api/projects?profile="+work.ID, map[string]string{"path": folder, "name": "Same"}))

	list := flatProjects(t, s)
	a, b := flatNamed(t, list, "Same", "Default"), flatNamed(t, list, "Same", "Work")
	if a.ID == b.ID || a.ID == home.ID || b.ID == away.ID {
		t.Fatalf("flat IDs must differ from each other and from the source IDs: %d %d", a.ID, b.ID)
	}
	for _, p := range list {
		// Work opens with its own orchestrator; only Default's is kept.
		if p.Kind == "orchestrator" {
			t.Fatalf("an added profile's orchestrator is listed: %+v", p)
		}
	}
	if list[0].Source.Profile != "Default" || a.Source.Remote {
		t.Fatalf("Default must come first: %+v", list)
	}

	// A task made from the flat view lands in the project's own profile.
	body := map[string]any{"project_id": b.ID, "provider": "fake", "model": "fake-quick", "title": "Over there"}
	resp := do(t, s, "POST", "/api/sessions?profile=all", body)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	task := decode[store.Session](t, resp)
	if task.ProjectID != b.ID {
		t.Fatalf("task project %d, want %d", task.ProjectID, b.ID)
	}
	if own := decode[[]store.Session](t, do(t, s, "GET", "/api/sessions?profile="+work.ID, nil)); len(own) != 1 || own[0].ProjectID != away.ID {
		t.Fatalf("task is not in Work: %+v", own)
	}
	if got := decode[[]store.Session](t, do(t, s, "GET", "/api/sessions?profile=all", nil)); len(got) != 1 || got[0].ID != task.ID {
		t.Fatalf("merged tasks: %+v", got)
	}
	if got := decode[[]store.Session](t, do(t, s, "GET", fmt.Sprintf("/api/sessions?profile=all&project_id=%d", a.ID), nil)); len(got) != 0 {
		t.Fatalf("Default's project has no tasks: %+v", got)
	}

	// A task found in no list yet is still found where it lives.
	s.flat.mu.Lock()
	s.flat.sessions = nil
	s.flat.mu.Unlock()
	resp = do(t, s, "PATCH", "/api/sessions/"+task.ID+"?profile=all", map[string]string{"title": "Renamed"})
	if resp.StatusCode != 200 {
		t.Fatalf("rename: %d", resp.StatusCode)
	}
	if got := decode[store.Session](t, resp); got.Title != "Renamed" || got.ProjectID != b.ID {
		t.Fatalf("renamed: %+v", got)
	}

	// Project routes reach the right folder and answer with the flat ID.
	got := decode[store.Project](t, do(t, s, "PATCH", fmt.Sprintf("/api/projects/%d?profile=all", b.ID), map[string]string{"name": "Renamed"}))
	if got.ID != b.ID || got.Name != "Renamed" {
		t.Fatalf("patched: %+v", got)
	}
	if own := decode[store.Project](t, do(t, s, "GET", fmt.Sprintf("/api/projects/%d?profile=%s", away.ID, work.ID), nil)); own.Name != "Renamed" {
		t.Fatalf("Work's project: %+v", own)
	}
	if resp := do(t, s, "GET", fmt.Sprintf("/api/projects/%d?profile=all", home.ID<<flatBits|(a.ID&flatMask^1)), nil); resp.StatusCode != 404 {
		t.Fatalf("unknown source: %d", resp.StatusCode)
	}

	// Favourites follow the profile of the project they are asked for.
	star := map[string]any{"provider": "fake", "model": "fake-quick"}
	if resp := do(t, s, "POST", fmt.Sprintf("/api/stars?profile=all&project_id=%d", b.ID), star); resp.StatusCode != 204 {
		t.Fatalf("star: %d", resp.StatusCode)
	}
	if got := decode[[]map[string]any](t, do(t, s, "GET", "/api/stars?profile="+work.ID, nil)); len(got) != 1 {
		t.Fatalf("Work's favourites: %+v", got)
	}
	if got := decode[[]map[string]any](t, do(t, s, "GET", fmt.Sprintf("/api/stars?profile=all&project_id=%d", a.ID), nil)); len(got) != 0 {
		t.Fatalf("Default's favourites: %+v", got)
	}

	// A dragged order mixes profiles and is the flat view's own: no
	// profile's order changes.
	do(t, s, "POST", "/api/projects", map[string]string{"path": t.TempDir(), "name": "Other"})
	before := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	list = flatProjects(t, s)
	o := flatNamed(t, list, "Other", "Default")
	resp = do(t, s, "POST", "/api/projects/order?profile=all", map[string]any{"ids": []int64{a.ID, b.ID, o.ID}})
	if resp.StatusCode != 204 {
		t.Fatalf("order: %d", resp.StatusCode)
	}
	if got := flatIDs(flatProjects(t, s)); fmt.Sprint(got) != fmt.Sprint([]int64{a.ID, b.ID, o.ID}) {
		t.Fatalf("flat order %v, want %v", got, []int64{a.ID, b.ID, o.ID})
	}
	own := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	if len(own) != 2 || own[0].ID != before[0].ID || own[1].ID != before[1].ID {
		t.Fatalf("Default order changed: %+v", own)
	}

	// Hiding from the flat view leaves the project in its profile, and
	// restoring puts it back where it was.
	resp = do(t, s, "PATCH", fmt.Sprintf("/api/projects/%d?profile=all", b.ID), map[string]bool{"hidden": true})
	if resp.StatusCode != 204 {
		t.Fatalf("hide: %d", resp.StatusCode)
	}
	if got := flatIDs(flatProjects(t, s)); fmt.Sprint(got) != fmt.Sprint([]int64{a.ID, o.ID}) {
		t.Fatalf("flat after hide %v", got)
	}
	hidden := decode[[]flatProject](t, do(t, s, "GET", "/api/projects?profile=all&hidden=true", nil))
	if len(hidden) != 1 || hidden[0].ID != b.ID || hidden[0].Name != "Renamed" || hidden[0].Source.Profile != "Work" {
		t.Fatalf("flat hidden: %+v", hidden)
	}
	if own := decode[[]store.Project](t, do(t, s, "GET", "/api/projects?profile="+work.ID, nil)); len(flatIDsOf(own)) != 1 {
		t.Fatalf("Work lost its project: %+v", own)
	}
	if got := decode[[]store.HiddenProject](t, do(t, s, "GET", "/api/projects?hidden=true&profile="+work.ID, nil)); len(got) != 0 {
		t.Fatalf("Work hid it too: %+v", got)
	}
	do(t, s, "PATCH", fmt.Sprintf("/api/projects/%d?profile=all", b.ID), map[string]bool{"hidden": false})
	if got := flatIDs(flatProjects(t, s)); fmt.Sprint(got) != fmt.Sprint([]int64{a.ID, b.ID, o.ID}) {
		t.Fatalf("flat after restore %v", got)
	}
}

// flatIDs lists the flat view's project IDs, leaving out the orchestrator.
func flatIDs(list []flatProject) []int64 {
	var ids []int64
	for _, p := range list {
		if p.Kind != "orchestrator" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func flatIDsOf(list []store.Project) []int64 {
	var ids []int64
	for _, p := range list {
		if p.Kind != "orchestrator" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func TestFlatArrangePlacesNewProjects(t *testing.T) {
	entry := func(id int64, kind string) flatEntry {
		return flatEntry{key: store.FlatProjectKey{Source: "/default", ID: id}, item: map[string]any{"id": id, "kind": kind}}
	}
	entries := []flatEntry{entry(1, "orchestrator"), entry(2, ""), entry(3, ""), entry(4, ""), entry(5, "")}
	saved := map[store.FlatProjectKey]store.FlatProject{
		{Source: "/default", ID: 2}: {Position: 2},
		{Source: "/default", ID: 3}: {Position: 1},
		{Source: "/default", ID: 4}: {Position: 1, HiddenAt: 9},
	}
	ids := func(list []flatEntry) []int64 {
		var out []int64
		for _, e := range list {
			out = append(out, e.key.ID)
		}
		return out
	}
	visible, hidden := flatArrange(entries, saved, false)
	if fmt.Sprint(ids(visible), ids(hidden)) != "[1 5 3 2] [4]" {
		t.Fatalf("new on top: %v %v", ids(visible), ids(hidden))
	}
	visible, _ = flatArrange(entries, saved, true)
	if fmt.Sprint(ids(visible)) != "[1 3 2 5]" {
		t.Fatalf("new at the bottom: %v", ids(visible))
	}
}

func TestFlatViewIncludesRemoteMachines(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	remoteSrv.store.SetServerName("Laptop") //nolint:errcheck
	made := decode[store.Project](t, do(t, remoteSrv, "POST", "/api/projects", map[string]string{"path": t.TempDir(), "name": "Far"}))
	address := listen(t, remoteSrv)
	local, _ := newTestServer(t)
	decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))

	list := flatProjects(t, local)
	far := flatNamed(t, list, "Far", "Default")
	if !far.Source.Remote || far.Source.Machine != "Laptop" || far.ID == made.ID {
		t.Fatalf("remote project: %+v", far)
	}
	got := decode[store.Project](t, do(t, local, "GET", fmt.Sprintf("/api/projects/%d?profile=all", far.ID), nil))
	if got.ID != far.ID || got.Name != "Far" {
		t.Fatalf("remote project route: %+v", got)
	}

	// The merged stream carries the remote's events, numbered the flat way.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + listen(t, local) + fmt.Sprintf("/api/stream?profile=all&projects=%d", far.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	if line, err := r.ReadString('\n'); err != nil || line != ": open\n" {
		t.Fatalf("first line %q: %v", line, err)
	}
	// Give the inner streams a moment to subscribe before publishing.
	deadline := time.Now().Add(3 * time.Second)
	go func() {
		for time.Now().Before(deadline) {
			remoteSrv.projectsChanged()
			time.Sleep(100 * time.Millisecond)
		}
	}()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("no remote event: %v", err)
		}
		if strings.Contains(line, "projects_changed") {
			break
		}
	}

	// Once the machine stops answering, lists still come back without it.
	remoteSrv.Shutdown()
	start := time.Now()
	for _, p := range flatProjects(t, local) {
		if p.Source.Remote {
			t.Fatalf("stopped remote listed: %+v", p)
		}
	}
	flatProjects(t, local)
	if time.Since(start) > 2*flatTimeout {
		t.Fatalf("lists waited %v on a stopped machine", time.Since(start))
	}
	_ = remoteStore
}

func TestFlatViewCopiesImagesToTheProfileThatUsesThem(t *testing.T) {
	remoteSrv, _ := newTestServer(t)
	address := listen(t, remoteSrv)
	local, _ := newTestServer(t)
	decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))

	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))) //nolint:errcheck
	resp, err := local.app.Test(httptest.NewRequest("POST", "/api/attachments?profile=all", bytes.NewReader(img.Bytes())))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %d", err, resp.StatusCode)
	}
	url := decode[struct{ URL string }](t, resp).URL
	if resp := do(t, remoteSrv, "GET", url, nil); resp.StatusCode != 404 {
		t.Fatalf("remote has it before use: %d", resp.StatusCode)
	}
	var src flatSource
	for _, candidate := range local.flatSources() {
		if candidate.Remote != "" {
			src = candidate
		}
	}
	if err := local.flatCopyAttachments(src, []byte(`{"prompt":"look ![Attached image](`+url+`)"}`)); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, remoteSrv, "GET", url, nil); resp.StatusCode != 200 {
		t.Fatalf("remote copy: %d", resp.StatusCode)
	}
	if resp := do(t, local, "GET", url+"?profile=all", nil); resp.StatusCode != 200 {
		t.Fatalf("flat read: %d", resp.StatusCode)
	}
}

func TestFlatCodesAreStableAndDistinct(t *testing.T) {
	var list []flatSource
	for i := range 2000 {
		list = append(list, flatSource{Remote: fmt.Sprintf("machine-%d", i/4), Profile: fmt.Sprintf("p%d", i%4)})
	}
	assignFlatCodes(list)
	seen := map[int64]bool{}
	for _, src := range list {
		if seen[src.code] {
			t.Fatalf("code %d used twice", src.code)
		}
		seen[src.code] = true
	}
	again := append([]flatSource{}, list[1000:]...)
	assignFlatCodes(again)
	moved := 0
	for i, src := range again {
		if src.code != list[1000+i].code {
			moved++
		}
	}
	// Removing sources only frees codes; a few that had collided may move.
	if moved > 20 {
		t.Fatalf("%d of %d codes moved", moved, len(again))
	}
}

func TestFlatOrchestratorNeedsSomethingToFlatten(t *testing.T) {
	s := profileServer(t)
	if _, ok := s.FlatProjectID(5); ok {
		t.Fatal("one profile has no flat view")
	}
	decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	id, ok := s.FlatProjectID(5)
	if !ok || id>>flatBits != 5 {
		t.Fatalf("flat orchestrator ID %d %v", id, ok)
	}
	// Added profiles keep their orchestrators to themselves.
	for _, rt := range s.profiles.running {
		if _, ok := rt.server.FlatProjectID(5); ok {
			t.Fatal("an added profile's orchestrator was given the flat view")
		}
	}
	if text := orchestrator.Context("agenttik", "/data", "task", 5, id); !strings.Contains(text, "profile=all") || !strings.Contains(text, fmt.Sprintf("Orchestrator project ID: %d", id)) {
		t.Fatalf("flat context: %s", text)
	}
	if text := orchestrator.Context("agenttik", "/data", "task", 5, 0); strings.Contains(text, "profile=all") {
		t.Fatalf("plain context mentions the flat view: %s", text)
	}
}

// A terminal's output stream goes to the profile that owns the shell, though
// the window names each terminal with the bytes it has drawn.
func TestFlatTerminalStreamFindsItsProfile(t *testing.T) {
	s := profileServer(t)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	do(t, s, "POST", "/api/projects?profile="+work.ID, map[string]string{"path": t.TempDir(), "name": "Away"})
	away := flatNamed(t, flatProjects(t, s), "Away", "Work")

	resp := do(t, s, "POST", fmt.Sprintf("/api/projects/%d/terminals?profile=all", away.ID), map[string]int{"cols": 80, "rows": 24})
	if resp.StatusCode != 200 {
		t.Skipf("no pseudo-terminal available here: %d", resp.StatusCode)
	}
	opened := decode[terminals.Info](t, resp)

	sources := s.flatSources()
	req := flatRequest{path: "/api/stream/terminals", query: url.Values{"ids": {opened.ID + ":42"}}}
	src, err := s.flatLocate(sources, &req, []string{"stream", "terminals"})
	if err != nil || src.Name != "Work" {
		t.Fatalf("stream went to %q: %v", src.Name, err)
	}
	if code := do(t, s, "DELETE", "/api/terminals/"+opened.ID+"?profile=all", nil).StatusCode; code != 204 {
		t.Errorf("closing from the flat view answered %d", code)
	}
}

// The flat view asks for the screen of the project in front, and the screen
// still sees where the browser is, though this machine's profiles are reached
// in process.
func TestFlatScreenKnowsTheBrowser(t *testing.T) {
	s := profileServer(t)
	fakeScreen(t, s)
	work := decode[Profile](t, do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"}))
	do(t, s, "POST", "/api/projects?profile="+work.ID, map[string]string{"path": t.TempDir(), "name": "Away"})
	away := flatNamed(t, flatProjects(t, s), "Away", "Work")

	get := func(forwarded string) screenInfo {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/screen?profile=all&project_id=%d", away.ID), nil)
		req.Header.Set("X-Forwarded-For", forwarded)
		resp, err := s.app.Test(req, 5000)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("screen: %v %v", resp.StatusCode, err)
		}
		return decode[screenInfo](t, resp)
	}
	if get("192.168.1.20").SameMachine {
		t.Error("a phone on the network is another machine")
	}
	if !get("127.0.0.1").SameMachine {
		t.Error("a browser on this machine is this machine")
	}
}
