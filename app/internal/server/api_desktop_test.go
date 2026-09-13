package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestDesktopConfigAPI(t *testing.T) {
	s, _ := newTestServer(t)
	response := do(t, s, "GET", "/api/desktop", nil)
	var initial struct {
		store.DesktopConfig
		Available bool `json:"available"`
	}
	if err := json.NewDecoder(response.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if initial.Available || initial.CloseToTray || initial.ToggleShortcut != "Ctrl+Shift+A" {
		t.Fatalf("defaults: %+v", initial)
	}
	response = do(t, s, "PUT", "/api/desktop", store.DesktopConfig{CloseToTray: true, ToggleShortcut: "Ctrl+Shift+B"})
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal("web mode accepted tray settings")
	}
	config, err := s.ConfigureDesktop(func() string { return "shortcut already taken" })
	if err != nil || config != initial.DesktopConfig {
		t.Fatalf("startup: %+v %v", config, err)
	}
	for _, step := range []struct {
		chord  string
		status int
	}{
		{"Ctrl+Shift+B", http.StatusOK},
		{"Cmd+Shift+B", http.StatusOK},
		{"A", http.StatusBadRequest},
		{"Ctrl+Ctrl+A", http.StatusBadRequest},
		{"Ctrl+F13", http.StatusBadRequest},
		{"Ctrl+Q", http.StatusBadRequest},
	} {
		response = do(t, s, "PUT", "/api/desktop", store.DesktopConfig{CloseToTray: true, ToggleShortcut: step.chord})
		response.Body.Close()
		if response.StatusCode != step.status {
			t.Fatalf("%s: %d", step.chord, response.StatusCode)
		}
	}
	response = do(t, s, "GET", "/api/desktop", nil)
	defer response.Body.Close()
	var got struct {
		store.DesktopConfig
		Available bool   `json:"available"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || !got.CloseToTray || got.ToggleShortcut != "Cmd+Shift+B" || got.Error != "shortcut already taken" {
		t.Fatalf("saved: %+v", got)
	}
}
