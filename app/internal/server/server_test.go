package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/web"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry()
	return New(st, reg, runner.New(st, reg, runner.NewHub())), st
}

func do(t *testing.T, s *Server, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		r = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestCreateProjectRejectsNonDirectory(t *testing.T) {
	s, _ := newTestServer(t)
	resp := do(t, s, "POST", "/api/projects", map[string]string{"path": "/definitely/not/here"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateProjectDefaultsNameToFolder(t *testing.T) {
	s, _ := newTestServer(t)
	dir := filepath.Join(t.TempDir(), "myproj")
	os.Mkdir(dir, 0o755)

	resp := do(t, s, "POST", "/api/projects", map[string]string{"path": dir})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	p := decode[store.Project](t, resp)
	if p.Name != "myproj" {
		t.Errorf("name = %q, want myproj", p.Name)
	}
}

func TestSessionRequiresKnownProvider(t *testing.T) {
	s, st := newTestServer(t)
	p, _ := st.CreateProject("alpha", t.TempDir())
	resp := do(t, s, "POST", "/api/sessions",
		map[string]any{"project_id": p.ID, "provider": "nope"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUnknownWindowRejected(t *testing.T) {
	s, _ := newTestServer(t)
	resp := do(t, s, "GET", "/api/sessions?window=99y", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMissingSessionIs404(t *testing.T) {
	s, _ := newTestServer(t)
	resp := do(t, s, "GET", "/api/sessions/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestResolveInRootRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hi"), 0o644)

	if _, err := resolveInRoot(root, "ok.txt"); err != nil {
		t.Errorf("ok.txt should resolve: %v", err)
	}
	for _, bad := range []string{"../escape", "../../etc/passwd", "/etc/passwd", ""} {
		if _, err := resolveInRoot(root, bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	// A symlink pointing outside the project is caught too.
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("s3cret"), 0o600)
	os.Symlink(outside, filepath.Join(root, "link"))
	if _, err := resolveInRoot(root, "link"); err == nil {
		t.Error("symlink out of the project should be rejected")
	}
}

func TestFileEndpointBlocksTraversal(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	p, _ := st.CreateProject("alpha", dir)

	resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/file?path=a.txt", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := decode[fileContent](t, resp); got.Content != "hello" {
		t.Errorf("content = %q", got.Content)
	}

	resp = do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/file?path=../../etc/passwd", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("traversal status = %d, want 400", resp.StatusCode)
	}
}

func TestBinaryFileIsNotInlined(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.bin"), []byte{0x00, 0x01, 0x02}, 0o644)
	p, _ := st.CreateProject("alpha", dir)

	got := decode[fileContent](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/file?path=b.bin", nil))
	if !got.Binary || got.Content != "" {
		t.Errorf("got %+v, want binary with no content", got)
	}
}

func TestTreeListsFiles(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "x", "y.js"), []byte("y"), 0o644)
	p, _ := st.CreateProject("alpha", dir)

	got := decode[projectFiles](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/tree", nil))
	if len(got.Files) != 2 || got.Files[0] != "a.txt" || got.Files[1] != "node_modules/x/y.js" {
		t.Errorf("tree = %v, want [a.txt node_modules/x/y.js]", got.Files)
	}
	// Nothing is ignored outside a repository: there is no .gitignore to read.
	if len(got.Ignored) != 0 {
		t.Errorf("ignored = %v, want none in a plain folder", got.Ignored)
	}
}

func TestParseStatus(t *testing.T) {
	out := " M internal/x.go\n?? new.txt\nR  old.go -> new.go\n"
	got := parseStatus(out)
	want := []changedFile{
		{Path: "internal/x.go", Status: "M"},
		{Path: "new.txt", Status: "??"},
		{Path: "new.go", Status: "R"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The UI is a Vite build, so a checkout that has not run `make ui` has nothing
// to serve. That is a build state, not a failure: the server answers with a
// notice saying so, and either answer is correct here.
func TestIndexIsServed(t *testing.T) {
	s, _ := newTestServer(t)
	resp := do(t, s, "GET", "/", nil)

	want := http.StatusOK
	if !web.Built() {
		want = http.StatusServiceUnavailable
	}
	if resp.StatusCode != want {
		t.Errorf("status = %d, want %d (ui built: %v)", resp.StatusCode, want, web.Built())
	}
}

func TestAPIResponsesAreCompressed(t *testing.T) {
	s, st := newTestServer(t)
	if _, err := st.CreateProject(strings.Repeat("project ", 100), t.TempDir()); err != nil {
		t.Fatalf("create project: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := s.app.Test(req, 5000)
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", got)
	}
	plain, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("open gzip body: %v", err)
	}
	defer plain.Close()
	var projects []store.Project
	if err := json.NewDecoder(plain).Decode(&projects); err != nil {
		t.Fatalf("decode gzip body: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
}

func TestStreamPathSkipsCompression(t *testing.T) {
	if !isStreamPath("/api/stream") {
		t.Error("the stream path should skip compression")
	}
	if isStreamPath("/api/projects") {
		t.Error("an ordinary API path should be compressed")
	}
}

// A project with no sessions must still carry an empty list: the sidebar
// iterates the field directly.
func TestProjectListAlwaysCarriesRecentSessions(t *testing.T) {
	s, st := newTestServer(t)
	st.CreateProject("alpha", t.TempDir())

	var got []map[string]any
	body, _ := io.ReadAll(do(t, s, "GET", "/api/projects", nil).Body)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	refs, ok := got[0]["recent_sessions"].([]any)
	if !ok {
		t.Fatalf("recent_sessions = %#v, want an array", got[0]["recent_sessions"])
	}
	if len(refs) != 0 {
		t.Errorf("recent_sessions = %v, want empty", refs)
	}
}

func TestHiddenProjectAPIListsNamesAndRestoresOrder(t *testing.T) {
	s, st := newTestServer(t)
	a, _ := st.CreateProject("alpha", t.TempDir())
	b, _ := st.CreateProject("beta", t.TempDir())
	c, _ := st.CreateProject("charlie", t.TempDir())
	d, _ := st.CreateProject("delta", t.TempDir())

	resp := do(t, s, "POST", "/api/projects/order", map[string]any{
		"ids": []int64{a.ID, b.ID, c.ID, d.ID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("order status = %d, want 204", resp.StatusCode)
	}

	resp = do(t, s, "PATCH", "/api/projects/"+itoa(c.ID), map[string]bool{"hidden": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hide status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	visible := decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	if got := serverProjectIDs(visible); !sameProjectIDs(got, []int64{a.ID, b.ID, d.ID}) {
		t.Fatalf("visible projects = %v, want alpha beta delta", got)
	}
	hidden := decode[[]store.HiddenProject](t, do(t, s, "GET", "/api/projects?hidden=true", nil))
	if len(hidden) != 1 || hidden[0] != (store.HiddenProject{ID: c.ID, Name: "charlie"}) {
		t.Fatalf("hidden projects = %+v, want id/name for charlie", hidden)
	}
	if hidden[0].Name == "" {
		t.Fatal("hidden project response did not include a name")
	}

	resp = do(t, s, "POST", "/api/projects/order", map[string]any{
		"ids": []int64{d.ID, a.ID, b.ID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reorder while hidden status = %d, want 204", resp.StatusCode)
	}
	resp = do(t, s, "PATCH", "/api/projects/"+itoa(c.ID), map[string]bool{"hidden": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore status = %d, want 200", resp.StatusCode)
	}
	visible = decode[[]store.Project](t, do(t, s, "GET", "/api/projects", nil))
	if got := serverProjectIDs(visible); !sameProjectIDs(got, []int64{d.ID, a.ID, c.ID, b.ID}) {
		t.Fatalf("visible after restore = %v, want delta alpha charlie beta", got)
	}
}

func serverProjectIDs(projects []store.Project) []int64 {
	ids := make([]int64, len(projects))
	for i, project := range projects {
		ids[i] = project.ID
	}
	return ids
}

func sameProjectIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The folder picker browses outside any project, so these check the listing
// shape rather than containment.
func TestBrowseListsDirectoriesOnly(t *testing.T) {
	s, _ := newTestServer(t)
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "beta"), 0o755)
	os.Mkdir(filepath.Join(root, "Alpha"), 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644)

	resp := do(t, s, "GET", "/api/fs?path="+root, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decode[dirListing](t, resp)
	if len(got.Dirs) != 2 {
		t.Fatalf("dirs = %+v, want 2 entries", got.Dirs)
	}
	// Case-insensitive order, so "Alpha" is not banished above "beta".
	if got.Dirs[0].Name != "Alpha" || got.Dirs[1].Name != "beta" {
		t.Errorf("dirs = %+v, want Alpha then beta", got.Dirs)
	}
	if got.Path != root || got.Parent != filepath.Dir(root) {
		t.Errorf("path = %q parent = %q", got.Path, got.Parent)
	}
	if len(got.Crumbs) == 0 || got.Crumbs[0].Path != string(filepath.Separator) {
		t.Errorf("crumbs = %+v, want root first", got.Crumbs)
	}
}

func TestBrowseHidesDotDirsUnlessAsked(t *testing.T) {
	s, _ := newTestServer(t)
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".config"), 0o755)

	got := decode[dirListing](t, do(t, s, "GET", "/api/fs?path="+root, nil))
	if len(got.Dirs) != 0 {
		t.Errorf("dirs = %+v, want dot folders hidden", got.Dirs)
	}
	got = decode[dirListing](t, do(t, s, "GET", "/api/fs?path="+root+"&hidden=true", nil))
	if len(got.Dirs) != 1 || got.Dirs[0].Name != ".config" {
		t.Errorf("dirs = %+v, want .config", got.Dirs)
	}
}

func TestBrowseRejectsFiles(t *testing.T) {
	s, _ := newTestServer(t)
	file := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(file, []byte("x"), 0o644)

	resp := do(t, s, "GET", "/api/fs?path="+file, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// A live SSE stream never ends by itself. Fiber waits for every open
// connection, so without releasing the streams first the window's close button
// could not quit the app.
func TestShutdownReturnsWithOpenStream(t *testing.T) {
	s, st := newTestServer(t)
	p, _ := st.CreateProject("alpha", t.TempDir())
	if err := st.CreateSession(&store.Session{ID: "s1", ProjectID: p.ID, Provider: "claude"}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go s.Listener(ln)

	var resp *http.Response
	for i := 0; i < 50 && resp == nil; i++ {
		resp, err = http.Get("http://" + ln.Addr().String() + "/api/stream?sessions=s1")
		if err != nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if resp == nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()

	// Shutdown must release the stream itself, not fall back on its timeout,
	// so it has to return cleanly and well inside the deadline.
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- s.Shutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		if elapsed := time.Since(start); elapsed >= shutdownTimeout {
			t.Fatalf("Shutdown took %v, want well under the %v timeout", elapsed, shutdownTimeout)
		}
	case <-time.After(shutdownTimeout + 3*time.Second):
		t.Fatal("Shutdown blocked while an SSE stream was open")
	}
}

func itoa(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestProjectStatsAndRename(t *testing.T) {
	s, st := newTestServer(t)
	p, _ := st.CreateProject("alpha", t.TempDir())
	if err := st.CreateSession(&store.Session{ID: "s1", ProjectID: p.ID, Provider: "claude", Model: "m"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turn, _ := st.StartTurn("s1", "m", "")
	turn.OutputTokens, turn.Status = 7, "ok"
	st.FinishTurn(turn)

	got := decode[store.ProjectStats](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/stats", nil))
	if got.Sessions != 1 || got.Turns != 1 || got.OutputTokens != 7 {
		t.Errorf("stats = %+v", got)
	}
	if resp := do(t, s, "GET", "/api/projects/999/stats", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown project status = %d, want 404", resp.StatusCode)
	}

	resp := do(t, s, "PATCH", "/api/projects/"+itoa(p.ID), map[string]string{"name": " beta "})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, want 200", resp.StatusCode)
	}
	if renamed := decode[store.Project](t, resp); renamed.Name != "beta" {
		t.Errorf("name = %q, want beta", renamed.Name)
	}
	if resp := do(t, s, "PATCH", "/api/projects/"+itoa(p.ID), map[string]string{"name": ""}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name status = %d, want 400", resp.StatusCode)
	}
}

// A project topic carries the end of every turn run in it, so a project view
// keeps its totals current while several of its sessions run at once.
func TestProjectStreamDeliversTurnEvents(t *testing.T) {
	s, st := newTestServer(t)
	p, _ := st.CreateProject("alpha", t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go s.Listener(ln)
	t.Cleanup(func() { s.Shutdown() })

	url := "http://" + ln.Addr().String() + "/api/stream?projects=" + itoa(p.ID)
	var resp *http.Response
	for i := 0; i < 50 && resp == nil; i++ {
		if resp, err = http.Get(url); err != nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if resp == nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", ct)
	}

	lines := make(chan string, 8)
	go func() {
		r := bufio.NewReader(resp.Body)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- line
		}
	}()

	// The handler subscribes before it writes anything, so the opening
	// comment means the publish below cannot be missed.
	if open := <-lines; !strings.HasPrefix(open, ": open") {
		t.Fatalf("first line = %q, want the opening comment", open)
	}
	s.runner.Hub().Publish(runner.ProjectTopic(p.ID),
		runner.Event{SessionID: "s1", Stats: &store.Stats{Turns: 3}})

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before the event arrived")
			}
			if !strings.HasPrefix(line, "data: ") {
				continue // blank separator or a heartbeat comment
			}
			var got runner.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &got); err != nil {
				t.Fatalf("decode %q: %v", line, err)
			}
			if got.SessionID != "s1" || got.Stats == nil || got.Stats.Turns != 3 {
				t.Fatalf("event = %+v, want session s1 with 3 turns", got)
			}
			return
		case <-time.After(3 * time.Second):
			t.Fatal("no event on the project stream")
		}
	}
}
