package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestGeneralConfigAPI(t *testing.T) {
	s, _ := newTestServer(t)
	for _, step := range []struct {
		method, value string
		status        int
		want          string
	}{
		{"GET", "", http.StatusOK, "top"},
		{"PUT", "bottom", http.StatusOK, "bottom"},
		{"PUT", "invalid", http.StatusBadRequest, ""},
		{"GET", "", http.StatusOK, "bottom"},
		{"PUT", "top", http.StatusOK, "top"},
	} {
		var body any
		if step.method == "PUT" {
			body = map[string]string{"new_item_position": step.value}
		}
		resp := do(t, s, step.method, "/api/general", body)
		if resp.StatusCode != step.status {
			t.Fatalf("%s %s: status %d", step.method, step.value, resp.StatusCode)
		}
		if step.want != "" {
			var cfg store.GeneralConfig
			if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.NewItemPosition != step.want {
				t.Fatalf("position = %q, want %q", cfg.NewItemPosition, step.want)
			}
		}
		resp.Body.Close()
	}
}
