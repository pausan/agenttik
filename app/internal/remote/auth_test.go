package remote_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/remote"
	"golang.org/x/crypto/bcrypt"
)

func TestProtectedConnection(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	secret, _ := netauth.NewSecret()
	gate := netauth.New(func() netauth.Credentials {
		return netauth.Credentials{Enabled: true, PasswordHash: string(hash), Secret: secret}
	})
	srv := httptest.NewServer(gate.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == remote.VersionPath {
			json.NewEncoder(w).Encode(remote.Info{Application: "agenttik", Version: "test"})
			return
		}
		io.WriteString(w, "signed in")
	})))
	defer srv.Close()
	target, _, err := remote.Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := remote.NewClient(http.NotFoundHandler())
	client.Connect(target)
	get := func() string {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		client.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if !strings.Contains(get(), "password") {
		t.Fatal("missing login form")
	}
	login := func(password, code string) int {
		body := url.Values{"password": {password}, "code": {code}, "next": {"/"}}
		req := httptest.NewRequest("POST", "/__auth/login", strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		client.ServeHTTP(rec, req)
		return rec.Code
	}
	code, _ := netauth.Code(secret, time.Now())
	if status := login("wrong", code); status != 401 {
		t.Fatalf("wrong password: %d", status)
	}
	if status := login("test-password", code); status != 303 {
		t.Fatalf("login: %d", status)
	}
	if got := get(); got != "signed in" {
		t.Fatal(got)
	}
	gate.Revoke()
	if !strings.Contains(get(), "password") {
		t.Fatal("revoked session still works")
	}
}

func TestProtectedConnectionPreviousCode(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password &+= spaces"), bcrypt.MinCost)
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	gate := netauth.New(func() netauth.Credentials {
		return netauth.Credentials{Enabled: true, PasswordHash: string(hash), Secret: secret}
	})
	srv := httptest.NewServer(gate.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "signed in")
	})))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	client := remote.NewClient(http.NotFoundHandler())
	client.Connect(target)
	code, _ := netauth.Code(secret, time.Now().Add(-netauth.Period))
	body := url.Values{"password": {"password &+= spaces"}, "code": {code}, "next": {"/"}}
	req := httptest.NewRequest("POST", "/__auth/login", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	client.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("previous code login: %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	client.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "signed in" {
		t.Fatal("remote session was not retained")
	}
}
