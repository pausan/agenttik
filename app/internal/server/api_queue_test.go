package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestDeleteQueuedMessage(t *testing.T) {
	s, st := newTestServer(t)
	p, err := st.CreateProject("alpha", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"owner", "other"} {
		if err := st.CreateSession(&store.Session{ID: id, ProjectID: p.ID, Provider: "fake"}); err != nil {
			t.Fatal(err)
		}
	}
	q, err := st.EnqueueMessage("owner", "delete me", "fake", 0, "m1", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/sessions/owner/queue/nope", http.StatusBadRequest},
		{"/api/sessions/owner/queue/0", http.StatusBadRequest},
		{"/api/sessions/owner/queue/-1", http.StatusBadRequest},
		{fmt.Sprintf("/api/sessions/missing/queue/%d", q.ID), http.StatusNotFound},
		{fmt.Sprintf("/api/sessions/other/queue/%d", q.ID), http.StatusNotFound},
		{fmt.Sprintf("/api/sessions/owner/queue/%d", q.ID), http.StatusNoContent},
		{fmt.Sprintf("/api/sessions/owner/queue/%d", q.ID), http.StatusNotFound},
	} {
		resp := do(t, s, "DELETE", tc.path, nil)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("DELETE %s = %d, want %d", tc.path, resp.StatusCode, tc.want)
		}
	}
	detail := decode[sessionDetail](t, do(t, s, "GET", "/api/sessions/owner", nil))
	if len(detail.Queued) != 0 || detail.Session.QueueCount != 0 || len(detail.Messages) != 0 {
		t.Fatalf("detail after deletion = %+v", detail)
	}
}
