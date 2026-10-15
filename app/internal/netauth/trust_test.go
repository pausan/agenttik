package netauth

import (
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// memTrust is a trusted list kept in a map.
type memTrust struct {
	mu       sync.Mutex
	machines map[string]Machine
}

func (m *memTrust) Visit(x Machine) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.machines[x.Key]; !ok {
		return false
	}
	m.machines[x.Key] = x
	return true
}

func (m *memTrust) Add(x Machine) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.machines[x.Key] = x
	return nil
}

func trusted(t *testing.T) (*Gate, http.Handler, *memTrust, ed25519.PrivateKey) {
	t.Helper()
	g, h := gated(testCreds())
	trust := &memTrust{machines: map[string]Machine{}}
	g.UseTrust("server-1", trust)
	key, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "machine.key"))
	if err != nil {
		t.Fatal(err)
	}
	return g, h, trust, key
}

// keyLogin asks for a nonce and answers it signed for serverID.
func keyLogin(t *testing.T, h http.Handler, key ed25519.PrivateKey, serverID string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, ChallengePath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var c struct{ Nonce string }
	if w.Code != 200 || json.NewDecoder(w.Body).Decode(&c) != nil || c.Nonce == "" {
		t.Fatalf("challenge: %d %s", w.Code, w.Body)
	}
	return postKey(h, key, c.Nonce, Sign(key, serverID, c.Nonce))
}

func postKey(h http.Handler, key ed25519.PrivateKey, nonce, sig string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{
		"key": EncodeKey(key.Public().(ed25519.PublicKey)), "nonce": nonce, "signature": sig, "id": "client-1", "name": "laptop",
	})
	r := httptest.NewRequest(http.MethodPost, KeyPath, strings.NewReader(string(body)))
	r.RemoteAddr = "192.168.1.20:4000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// withKey adds the fields an agenttik client sends with its password.
func withKey(key ed25519.PrivateKey) func(*http.Request) {
	return func(r *http.Request) {
		r.ParseForm()
		form := r.PostForm
		form.Set("key", EncodeKey(key.Public().(ed25519.PublicKey)))
		form.Set("id", "client-1")
		form.Set("name", "laptop")
		r.Body = io.NopCloser(strings.NewReader(form.Encode()))
		r.PostForm, r.Form = nil, nil
	}
}

// previousCode is the code of the step before, which is still accepted, so a
// test can sign in twice without waiting for the next step.
func previousCode(t *testing.T) string {
	t.Helper()
	code, err := Code(rfcSecret, time.Now().Add(-Period))
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestKeySignInNeedsAPasswordSignInFirst(t *testing.T) {
	g, h, trust, key := trusted(t)
	if w := keyLogin(t, h, key, "server-1"); w.Code != http.StatusUnauthorized {
		t.Fatalf("untrusted key signed in: %d", w.Code)
	}

	// A browser's sign-in trusts nothing.
	if w := attempt(h, testPassword, liveCode(t)); w.Code != http.StatusSeeOther || len(trust.machines) != 0 {
		t.Fatalf("browser sign-in: %d %v", w.Code, trust.machines)
	}

	w := attempt(h, testPassword, previousCode(t), withKey(key))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("password sign-in: %d %s", w.Code, w.Body)
	}
	m := trust.machines[EncodeKey(key.Public().(ed25519.PublicKey))]
	if m.ID != "client-1" || m.Name != "laptop" || m.Address != "10.0.0.9" {
		t.Fatalf("trusted machine: %+v", m)
	}

	w = keyLogin(t, h, key, "server-1")
	if w.Code != http.StatusNoContent {
		t.Fatalf("key sign-in: %d %s", w.Code, w.Body)
	}
	cookie := sessionCookie(t, w)
	if got := get(h, "/", cookie); got.Body.String() != "behind" {
		t.Fatalf("key session refused: %d", got.Code)
	}
	if m := trust.machines[EncodeKey(key.Public().(ed25519.PublicKey))]; m.Address != "192.168.1.20" {
		t.Fatalf("visit not recorded from the new address: %+v", m)
	}

	// Changing a secret ends the session but not the trust.
	g.Revoke()
	if got := get(h, "/", cookie); got.Body.String() == "behind" {
		t.Fatal("session survived Revoke")
	}
	cookie = sessionCookie(t, keyLogin(t, h, key, "server-1"))

	// Removing the machine ends its session and its trust.
	delete(trust.machines, EncodeKey(key.Public().(ed25519.PublicKey)))
	g.RevokeKey(EncodeKey(key.Public().(ed25519.PublicKey)))
	if got := get(h, "/", cookie); got.Body.String() == "behind" {
		t.Fatal("session survived RevokeKey")
	}
	if w := keyLogin(t, h, key, "server-1"); w.Code != http.StatusUnauthorized {
		t.Fatalf("removed machine signed in: %d", w.Code)
	}
}

func TestKeySignInRefusesReplayOtherServersAndForgery(t *testing.T) {
	_, h, trust, key := trusted(t)
	trust.Add(Machine{Key: EncodeKey(key.Public().(ed25519.PublicKey))})

	// Signed for another server.
	if w := keyLogin(t, h, key, "server-2"); w.Code != http.StatusUnauthorized {
		t.Fatalf("signature for another server: %d", w.Code)
	}
	// A nonce never handed out.
	if w := postKey(h, key, "made-up", Sign(key, "server-1", "made-up")); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown nonce: %d", w.Code)
	}
	// Signed by another key.
	_, other, _ := ed25519.GenerateKey(nil)
	r := httptest.NewRequest(http.MethodGet, ChallengePath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var c struct{ Nonce string }
	json.NewDecoder(w.Body).Decode(&c)
	if w := postKey(h, key, c.Nonce, Sign(other, "server-1", c.Nonce)); w.Code != http.StatusUnauthorized {
		t.Fatalf("forged signature: %d", w.Code)
	}
	// That nonce was spent by the failed attempt.
	if w := postKey(h, key, c.Nonce, Sign(key, "server-1", c.Nonce)); w.Code != http.StatusUnauthorized {
		t.Fatalf("nonce reused: %d", w.Code)
	}
	// Failed key sign-ins do not lock the password form.
	for range maxFailures + 1 {
		keyLogin(t, h, key, "server-2")
	}
	if w := attempt(h, testPassword, liveCode(t)); w.Code != http.StatusSeeOther {
		t.Fatalf("password locked out by key failures: %d", w.Code)
	}
	if w := keyLogin(t, h, key, "server-1"); w.Code != http.StatusNoContent {
		t.Fatalf("good signature: %d", w.Code)
	}
}

func TestKeySignInNeedsAFinishedLock(t *testing.T) {
	creds := testCreds()
	creds.PasswordHash = ""
	g, h := gated(creds)
	trust := &memTrust{machines: map[string]Machine{}}
	g.UseTrust("server-1", trust)
	_, key, _ := ed25519.GenerateKey(nil)
	trust.Add(Machine{Key: EncodeKey(key.Public().(ed25519.PublicKey))})
	if w := keyLogin(t, h, key, "server-1"); w.Code != http.StatusUnauthorized {
		t.Fatalf("half-set lock opened: %d", w.Code)
	}
}

func TestLoadOrCreateKeyKeepsOneKeyPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine.key")
	first, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadOrCreateKey(path)
	if err != nil || !first.Equal(again) {
		t.Fatalf("key changed: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	os.WriteFile(path, []byte("junk"), 0600)
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("a damaged key was replaced silently")
	}
}
