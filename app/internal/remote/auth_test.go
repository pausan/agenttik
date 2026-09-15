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
	login := func(password string) int {
		code, _ := netauth.Code(secret, time.Now())
		body := url.Values{"password": {password}, "code": {code}, "next": {"/"}}
		req := httptest.NewRequest("POST", "/__auth/login", strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		client.ServeHTTP(rec, req)
		return rec.Code
	}
	if status := login("wrong"); status != 401 {
		t.Fatalf("wrong password: %d", status)
	}
	if status := login("test-password"); status != 303 {
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
