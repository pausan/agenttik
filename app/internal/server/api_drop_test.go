package server

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopDropIsReadOnlyByTheWindow(t *testing.T) {
	s, _ := newTestServer(t)
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "folder/sub"), 0o755)
	os.MkdirAll(filepath.Join(dir, "folder/empty"), 0o755)
	os.WriteFile(filepath.Join(dir, "folder/sub/a.txt"), []byte("abcdef"), 0o644)
	os.WriteFile(filepath.Join(dir, "one.json"), []byte(`{"x":1}`), 0o644)
	os.WriteFile(filepath.Join(dir, "secret"), []byte("no"), 0o644)
	s.DropFiles([]string{filepath.Join(dir, "folder"), filepath.Join(dir, "one.json")})

	get := func(path, token string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		if token != "" {
			req.Header.Set("X-Agenttik-Window-Token", token)
		}
		resp, err := s.app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	// Nothing answers before the desktop shell enables drops, nor without its token.
	if code, _ := get("/api/desktop/drop", ""); code != 403 {
		t.Fatalf("before enabling: %d", code)
	}
	token := s.EnableFileDrops()
	if code, _ := get("/api/desktop/drop", "wrong"); code != 403 {
		t.Fatalf("wrong token: %d", code)
	}

	code, body := get("/api/desktop/drop", token)
	want := `{"dirs":["folder/empty"],"files":[{"path":"folder/sub/a.txt","size":6},{"path":"one.json","size":7}]}`
	if code != 200 || body != want {
		t.Fatalf("listing: %d %s", code, body)
	}
	for path, want := range map[string]string{
		"/api/desktop/drop/file?path=folder/sub/a.txt&offset=2&length=3": "cde",
		"/api/desktop/drop/file?path=folder/sub/a.txt&offset=4&length=9": "ef",
		"/api/desktop/drop/file?path=one.json&offset=0&length=100":       `{"x":1}`,
	} {
		if code, body := get(path, token); code != 200 || body != want {
			t.Fatalf("%s: %d %q", path, code, body)
		}
	}
	for _, path := range []string{
		"/api/desktop/drop/file?path=folder/../secret&offset=0&length=2",
		"/api/desktop/drop/file?path=folder/../../etc/passwd&offset=0&length=2",
		"/api/desktop/drop/file?path=one.json&offset=0&length=99999999",
	} {
		if code, _ := get(path, token); code != 400 {
			t.Fatalf("%s: %d", path, code)
		}
	}
	if code, _ := get("/api/desktop/drop/file?path=secret&offset=0&length=2", token); code != 404 {
		t.Fatalf("undropped file: %d", code)
	}
}
