package server

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
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

	// A dragged order is saved on each profile for its own projects.
	other := decode[store.Project](t, do(t, s, "POST", "/api/projects", map[string]string{"path": t.TempDir(), "name": "Other"}))
	list = flatProjects(t, s)
	o := flatNamed(t, list, "Other", "Default")
	resp = do(t, s, "POST", "/api/projects/order?profile=all", map[string]any{"ids": []int64{b.ID, o.ID, a.ID}})
	if resp.StatusCode != 204 {
		t.Fatalf("order: %d", resp.StatusCode)
	}
	own := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	if len(own) != 2 || own[0].ID != other.ID || own[1].ID != home.ID {
		t.Fatalf("Default order: %+v", own)
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
