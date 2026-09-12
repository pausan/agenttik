package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/single"
)

func TestCallAPIUsesRunningInstanceAndStdin(t *testing.T) {
	const body = `{"prompt":"quotes: \"hello\"\nsecond line; $(literal)"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.RequestURI() != "/api/sessions/task/messages?q=a%20b" || string(data) != body || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s %s", r.Method, r.URL, data)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"id":42}`)
	}))
	defer server.Close()
	cfg := config.Config{DataDir: t.TempDir()}
	lock, err := single.Acquire(cfg.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	addr := strings.TrimPrefix(server.URL, "http://")
	if err := lock.Publish(addr); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := callAPI(cfg, "POST", "/api/sessions/task/messages?q=a%20b", "-", strings.NewReader(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"id":42}` || single.Addr(cfg.LockPath()) != addr {
		t.Fatalf("output = %s, address = %s", out.String(), single.Addr(cfg.LockPath()))
	}
	if _, err := os.Stat(cfg.DBPath()); !os.IsNotExist(err) {
		t.Fatal("client created a database")
	}
}

func TestCallAPIErrors(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	for _, tc := range []struct{ method, path, body, want string }{
		{"GET", "/api/projects", "", "not running"},
		{"TRACE", "/api/projects", "", "unsupported"},
		{"GET", "https://example.com/api/projects", "", "path"},
		{"GET", "//example.com/api/projects", "", "path"},
		{"GET", "/projects", "", "path"},
		{"POST", "/api/projects", "{bad", "valid JSON"},
	} {
		err := callAPI(cfg, tc.method, tc.path, tc.body, nil, io.Discard)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v", tc, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":"task is busy"}`)
	}))
	defer server.Close()
	if err := os.WriteFile(cfg.LockPath(), []byte(strings.TrimPrefix(server.URL, "http://")), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := callAPI(cfg, "PATCH", "/api/sessions/task", `{"done":true}`, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "409 Conflict: task is busy") || out.Len() != 0 {
		t.Fatalf("failed request: output=%q, error=%v", out.String(), err)
	}
}
