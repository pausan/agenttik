package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for input, want := range map[string]string{"example.com:7717": "http://example.com:7717", " https://example.com/ ": "https://example.com", "[::1]:7717": "http://[::1]:7717"} {
		u, err := Parse(input)
		if err != nil || u.String() != want {
			t.Fatalf("Parse(%q) = %v, %v", input, u, err)
		}
	}
	for _, input := range []string{"", "ftp://example.com", "http://user:pass@example.com", "http://example.com/path", "http://example.com?q=1", "http://example.com/#fragment", "localhost:0", "localhost:65536", "localhost:abc", "localhost:", "::1"} {
		if _, err := Parse(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"instance", `{"application":"agenttik","version":"1.2.3"}`, 200, true},
		{"other", `{"application":"other","version":"1"}`, 200, false},
		{"missing version", `{"application":"agenttik"}`, 200, false},
		{"html", `<html>login</html>`, 200, false},
		{"redirect", `{"application":"agenttik","version":"1"}`, 302, false},
		{"unauthorized", `{"application":"agenttik","version":"1"}`, 401, false},
		{"oversized", strings.Repeat(" ", 4097) + `{"application":"agenttik","version":"1"}`, 200, false},
		{"trailing content", `{"application":"agenttik","version":"1"} garbage`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != VersionPath {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			_, info, err := Check(context.Background(), srv.URL)
			if (err == nil) != tc.valid {
				t.Fatalf("info=%+v err=%v", info, err)
			}
			if calls != 1 {
				t.Fatalf("followed redirect: %d requests", calls)
			}
		})
	}
}

func TestClientSwitchAndCookies(t *testing.T) {
	newRemote := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == VersionPath {
				json.NewEncoder(w).Encode(Info{Application: "agenttik", Version: "1", Name: name})
				return
			}
			if r.Header.Get("Authorization") != "" {
				t.Error("forwarded authorization")
			}
			if r.URL.Path == "/login" {
				http.SetCookie(w, &http.Cookie{Name: "session", Value: name, Path: "/", HttpOnly: true})
				http.Redirect(w, r, "/", 303)
				return
			}
			if cookie, err := r.Cookie("session"); err == nil && cookie.Value != name {
				t.Errorf("leaked cookie: %s", cookie.Value)
			}
			fmt.Fprintf(w, "%s:%s", name, r.Header.Get("Cookie"))
		}))
	}
	a, b := newRemote("a"), newRemote("b")
	defer a.Close()
	defer b.Close()
	client := NewClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "local") }))
	connect := func(address string, want int) {
		req := httptest.NewRequest("POST", ConnectPath, strings.NewReader(fmt.Sprintf(`{"address":%q}`, address)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		before := client.handler.Load()
		client.ServeHTTP(rec, req)
		if client.handler.Load() != before {
			t.Fatal("target changed before navigation")
		}
		if rec.Code != want {
			t.Fatalf("connect: %d %s", rec.Code, rec.Body.String())
		}
		if want == 200 {
			navigation := httptest.NewRequest("GET", "/", nil)
			navigation.Header.Set("Accept", "text/html")
			client.ServeHTTP(httptest.NewRecorder(), navigation)
		}
	}
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Cookie", "session=local-secret")
		req.Header.Set("Authorization", "Bearer local-secret")
		rec := httptest.NewRecorder()
		client.ServeHTTP(rec, req)
		return rec
	}
	connect(a.URL, 200)
	if got := get("/login"); got.Code != 303 || got.Header().Get("Set-Cookie") != "" {
		t.Fatalf("login: %v", got)
	}
	if got := get("/").Body.String(); got != "a:session=a" {
		t.Fatal(got)
	}
	connect("ftp://invalid", 400)
	if got := get("/").Body.String(); got != "a:session=a" {
		t.Fatal("failed check changed target: " + got)
	}
	connect(b.URL, 200)
	if got := get("/").Body.String(); got != "b:" {
		t.Fatal(got)
	}
}

func TestConnectRejectsCrossOriginAndForms(t *testing.T) {
	h := ConnectHandler(func(_ *url.URL) string { t.Fatal("connected"); return "" })
	for _, tc := range []struct {
		contentType, origin string
		status              int
	}{
		{"text/plain", "", 415}, {"application/json", "https://other.example", 403},
	} {
		req := httptest.NewRequest("POST", "http://localhost"+ConnectPath, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", tc.contentType)
		req.Header.Set("Origin", tc.origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("got %d", rec.Code)
		}
	}
}

func TestPreviewDoesNotSwitchClient(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Info{Application: "agenttik", Version: "1", Name: "My server"})
	}))
	defer target.Close()
	client := NewClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "local") }))
	req := httptest.NewRequest("POST", CheckPath, strings.NewReader(fmt.Sprintf(`{"address":%q}`, target.URL)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	client.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"My server"`) {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	client.ServeHTTP(rec, req)
	if rec.Body.String() != "local" {
		t.Fatal("preview switched the active server")
	}
}
