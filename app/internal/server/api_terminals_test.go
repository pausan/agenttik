package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/terminals"
)

// raw posts a body that is not JSON, which is how keystrokes travel.
func raw(t *testing.T, s *Server, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.app.Test(req, 5000)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// A terminal opens in the project's own folder, is listed there, and is gone
// once closed. The rest of the shell's behaviour is the terminals package's
// own test; this is the wiring.
func TestTerminalRoutes(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("alpha", dir)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/terminals", p.ID)

	resp := do(t, s, "POST", path, map[string]int{"cols": 100, "rows": 30})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Skipf("no pseudo-terminal available here: %s", body)
	}
	var opened terminals.Info
	if err := json.NewDecoder(resp.Body).Decode(&opened); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.terminals.Close(opened.ID) })
	if opened.ID == "" || opened.ProjectID != p.ID {
		t.Fatalf("opened terminal is not the project's: %+v", opened)
	}
	if opened.Title == "" {
		t.Error("a terminal tab needs a name")
	}

	var listed []terminals.Info
	body, _ := io.ReadAll(do(t, s, "GET", path, nil).Body)
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != opened.ID {
		t.Fatalf("the project should list the terminal it opened, got %s", body)
	}

	if code := raw(t, s, "/api/terminals/"+opened.ID+"/input", "echo hello\n").StatusCode; code != 204 {
		t.Errorf("typing into a terminal answered %d", code)
	}
	if code := do(t, s, "POST", "/api/terminals/"+opened.ID+"/resize", map[string]int{"cols": 120, "rows": 40}).StatusCode; code != 204 {
		t.Errorf("resizing a terminal answered %d", code)
	}
	if code := do(t, s, "DELETE", "/api/terminals/"+opened.ID, nil).StatusCode; code != 204 {
		t.Errorf("closing a terminal answered %d", code)
	}

	body, _ = io.ReadAll(do(t, s, "GET", path, nil).Body)
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("a closed terminal is still listed: %s", body)
	}
}

// A terminal that has gone is a 404 rather than a 500, so the UI can drop its
// tab instead of showing an error.
func TestTerminalGoneIsNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	if code := raw(t, s, "/api/terminals/nope/input", "x").StatusCode; code != 404 {
		t.Errorf("typing into a terminal that is gone answered %d, want 404", code)
	}
	if code := do(t, s, "DELETE", "/api/terminals/nope", nil).StatusCode; code != 404 {
		t.Errorf("closing a terminal that is gone answered %d, want 404", code)
	}
}

// Terminal output must not be buffered by the compressor on its way out, for
// the same reason the event stream is not.
func TestTerminalStreamSkipsCompression(t *testing.T) {
	if !isStreamPath("/api/stream/terminals") {
		t.Error("the terminal stream path should skip compression")
	}
}
