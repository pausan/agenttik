package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdatesRequireLocalDesktopToken(t *testing.T) {
	s, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	token, err := s.EnableUpdates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, token string
		want                int
	}{
		{"GET", "/api/updates", "", 200},
		{"POST", "/api/updates/install", "", 403},
		{"POST", "/api/updates/ignore", "wrong", 403},
		{"POST", "/api/updates/install", token, 409},
		{"POST", "/api/updates/ignore", token, 409},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"version":"v99.0.0"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Agenttik-Update-Token", tc.token)
		res, err := s.app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Fatalf("%s %s: %d want %d", tc.method, tc.path, res.StatusCode, tc.want)
		}
	}
	req := httptest.NewRequest("POST", "/api/updates/install", strings.NewReader(`{"version":"v99.0.0"}`))
	req.Header.Set("X-Agenttik-Update-Token", token)
	res, err := s.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("missing JSON type: %d", res.StatusCode)
	}
}

func TestPrivateInstanceDisablesUpdates(t *testing.T) {
	s, _ := newTestServer(t)
	if err := s.EnableProfiles(true); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	token, err := s.EnableUpdates(context.Background())
	if err != nil || token != "" || s.updater != nil {
		t.Fatalf("private updater: %q %v", token, err)
	}
}
