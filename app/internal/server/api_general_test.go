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

func TestApprovalModeSettings(t *testing.T) {
	s, _ := newTestServer(t)
	cfg := decode[store.GeneralConfig](t, do(t, s, "GET", "/api/general", nil))
	if cfg.ApprovalMode != "timeout" || cfg.ApprovalTimeout != 30 {
		t.Fatalf("defaults = %+v", cfg)
	}
	for _, bad := range []map[string]any{
		{"approval_mode": "sometimes"},
		{"approval_timeout": 0},
		{"approval_timeout": 86401},
	} {
		if resp := do(t, s, "PUT", "/api/general", bad); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", bad, resp.StatusCode)
		}
	}
	cfg = decode[store.GeneralConfig](t, do(t, s, "PUT", "/api/general", map[string]any{"approval_mode": "wait", "approval_timeout": 90}))
	if cfg.ApprovalMode != "wait" || cfg.ApprovalTimeout != 90 || cfg.NewItemPosition != "top" {
		t.Fatalf("saved = %+v", cfg)
	}
	// A body naming one setting leaves the others as they were.
	cfg = decode[store.GeneralConfig](t, do(t, s, "PUT", "/api/general", map[string]any{"new_item_position": "bottom"}))
	if cfg.ApprovalMode != "wait" || cfg.ApprovalTimeout != 90 {
		t.Fatalf("partial update reset approvals: %+v", cfg)
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
