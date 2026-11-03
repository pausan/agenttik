package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestModesAPI(t *testing.T) {
	s, st := newTestServer(t)
	p, err := st.CreateProject("modes", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []map[string]string{{"title": "x"}, {"body": "x"}, {"title": " ", "body": "x"}} {
		if resp := do(t, s, "POST", "/api/modes", body); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("create %+v = %d", body, resp.StatusCode)
		}
	}
	resp := do(t, s, "POST", "/api/modes", map[string]string{"title": "Review", "body": "Review only."})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	review := decode[store.Mode](t, resp)
	if resp := do(t, s, "POST", "/api/modes", map[string]string{"title": "review", "body": "x"}); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate = %d", resp.StatusCode)
	}
	path := fmt.Sprintf("/api/modes/%d", review.ID)
	if resp := do(t, s, "PATCH", path, map[string]string{"body": " "}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body = %d", resp.StatusCode)
	}
	if resp := do(t, s, "PATCH", path, map[string]string{"body": "Read, then review."}); resp.StatusCode != http.StatusOK {
		t.Fatalf("edit = %d", resp.StatusCode)
	}
	// A standing prompt is a way of working, not a pasted document: both the
	// mode text and a project's own prompt stop at maxPromptChars.
	long := strings.Repeat("x", maxPromptChars+1)
	if resp := do(t, s, "POST", "/api/modes", map[string]string{"title": "Long", "body": long}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create over the limit = %d", resp.StatusCode)
	}
	if resp := do(t, s, "PATCH", path, map[string]string{"body": long}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("edit over the limit = %d", resp.StatusCode)
	}
	if m, _ := st.GetMode(review.ID); m.Body != "Read, then review." {
		t.Fatalf("rejected edit still saved: %+v", m)
	}
	if resp := do(t, s, "POST", "/api/modes", map[string]string{"title": "Limit", "body": strings.Repeat("x", maxPromptChars)}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create at the limit = %d", resp.StatusCode)
	}
	// Only new text is measured, so a mode stored before the limit can still
	// be renamed.
	legacy, err := st.CreateMode("Legacy", long)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := fmt.Sprintf("/api/modes/%d", legacy.ID)
	if resp := do(t, s, "PATCH", legacyPath, map[string]string{"title": "Renamed"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("rename over the limit = %d", resp.StatusCode)
	}
	if m, _ := st.GetMode(legacy.ID); m.Title != "Renamed" || m.Body != long {
		t.Fatalf("renamed mode = %q, %d characters", m.Title, len(m.Body))
	}
	if m, _ := st.GetMode(review.ID); m.Title != "Review" || m.Body != "Read, then review." {
		t.Fatalf("mode = %+v", m)
	}

	project := fmt.Sprintf("/api/projects/%d", p.ID)
	for _, id := range []int64{-1, 99} {
		if resp := do(t, s, "PATCH", project, map[string]int64{"mode_id": id}); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("project mode %d = %d", id, resp.StatusCode)
		}
	}
	if resp := do(t, s, "PATCH", project, map[string]int64{"mode_id": review.ID}); resp.StatusCode != http.StatusOK {
		t.Fatalf("project mode = %d", resp.StatusCode)
	}
	if resp := do(t, s, "PATCH", project, map[string]string{"name": "renamed", "prompt": long}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("project prompt over the limit = %d", resp.StatusCode)
	}
	// Refused before anything is written, so the name in the same request stands.
	if got, _ := st.GetProject(p.ID); got.Name != "modes" || got.Prompt != "" {
		t.Fatalf("rejected request still wrote: %+v", got)
	}

	sess := &store.Session{ID: "task", ProjectID: p.ID, Provider: "fake"}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, s, "PATCH", "/api/sessions/task", map[string]int64{"mode_id": store.ModeNone}); resp.StatusCode != http.StatusOK {
		t.Fatalf("task mode = %d", resp.StatusCode)
	}
	if _, err := st.EnqueueMessage("task", "first", "fake", 0, "m", ""); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, s, "PATCH", "/api/sessions/task", map[string]int64{"mode_id": review.ID}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mode after the first prompt = %d", resp.StatusCode)
	}

	if resp := do(t, s, "DELETE", path, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
	if got, _ := st.GetProject(p.ID); got.ModeID != 0 {
		t.Fatalf("project still linked: %d", got.ModeID)
	}
}
