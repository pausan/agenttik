package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/smartsearch"
)

func TestSmartSearchStatusAndValidation(t *testing.T) {
	s, _ := newTestServer(t)
	defer s.search.Close()
	status := decode[smartsearch.Status](t, do(t, s, "GET", "/api/smart-search", nil))
	if status.Phase != "idle" {
		t.Fatal(status)
	}
	for _, query := range []string{"", strings.Repeat("q", 16385)} {
		res := do(t, s, "POST", "/api/smart-search/query", map[string]any{"query": query, "ids": []string{"a"}})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatal(res.StatusCode)
		}
		res.Body.Close()
	}
	res := do(t, s, "POST", "/api/smart-search/query", map[string]any{"query": "login", "ids": []string{"a"}})
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatal(res.StatusCode)
	}
	if s.search.Status().Phase != "idle" {
		t.Fatal("query unexpectedly started model download")
	}
}
