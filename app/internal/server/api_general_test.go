package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

func TestGeneralDatabaseInfo(t *testing.T) {
	s, st := newTestServer(t)
	path := filepath.Join(st.Dir(), "t.db")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	resp := do(t, s, "GET", "/api/general", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got struct {
		Path string `json:"database_path"`
		Size int64  `json:"database_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Path != path || !filepath.IsAbs(got.Path) {
		t.Fatalf("path = %q, want %q", got.Path, path)
	}
	if got.Size != info.Size() || got.Size <= 0 {
		t.Fatalf("size = %d, want %d", got.Size, info.Size())
	}
}
