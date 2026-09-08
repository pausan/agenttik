package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pausan/agenttik/internal/agent"
	"github.com/pausan/agenttik/internal/runner"
	"github.com/pausan/agenttik/internal/store"
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

	paths := decode[[]string](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/tree", nil))
	if len(paths) != 1 || paths[0] != "a.txt" {
		t.Errorf("tree = %v, want [a.txt] with node_modules skipped", paths)
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

func TestIndexIsServed(t *testing.T) {
	s, _ := newTestServer(t)
	resp := do(t, s, "GET", "/", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func itoa(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
