package server

import (
	"bytes"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/netserver"
)

const exposedPassword = "correct horse battery"

// exposed starts the whole stack the way the desktop shell does — the app on
// a real loopback listener, and the extra listener reverse-proxying to it
// through the gate — and returns the address a browser would use. The
// exposure itself is started through the manager rather than the API so the
// test can take any free port; everything in front of it is the real thing.
func exposed(t *testing.T) (*Server, string) {
	t.Helper()
	s, _ := newTestServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go s.Listener(ln) //nolint:errcheck // closed by Shutdown
	t.Cleanup(func() { s.Shutdown() })

	m := netserver.New(ln.Addr().String())
	s.SetNetworkManager(m, "127.0.0.1", 7717)
	t.Cleanup(func() { m.Stop() })
	if err := m.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("expose: %v", err)
	}
	return s, "http://" + m.Status().Addr
}

// browser is an http.Client that keeps cookies and does not follow the
// redirect a good login answers with, so the test can see it.
func browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:           jar,
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func fetch(t *testing.T, c *http.Client, url, accept string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", accept)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// lockTheServer turns the lock on through the API the window itself uses, and
// returns the seed it rolled.
func lockTheServer(t *testing.T, s *Server) string {
	t.Helper()
	resp := do(t, s, "PUT", "/api/server/auth", map[string]any{
		"enabled": true, "password": exposedPassword,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lock the server: status %d", resp.StatusCode)
	}
	info := decode[serverConfigInfo](t, resp)
	if !info.AuthEnabled || !info.HasPassword {
		t.Fatalf("lock did not take: %+v", info)
	}
	if info.TOTPSecret == "" {
		t.Fatal("turning the lock on rolled no seed")
	}
	if !strings.Contains(info.TOTPURI, info.TOTPSecret) {
		t.Errorf("TOTPURI = %q, want it to carry the seed", info.TOTPURI)
	}
	return info.TOTPSecret
}

// TestExposedServerAsksForALoginAndTakesOne is the whole feature over real
// HTTP: locked out first, signed in after, on the address a browser uses.
func TestExposedServerAsksForALoginAndTakesOne(t *testing.T) {
	s, addr := exposed(t)
	secret := lockTheServer(t, s)
	c := browser()

	// A browser gets the form, and none of the app with it.
	resp := fetch(t, c, addr+"/", "text/html")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body := read(t, resp)
	if !strings.Contains(body, `name="password"`) || strings.Contains(body, "<script") {
		t.Errorf("body = %q, want the login form and no app assets", body)
	}

	// The UI's own fetches are refused with something they can act on.
	if resp := fetch(t, c, addr+"/api/projects", "*/*"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("api status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Sign in with a code the authenticator would be showing right now.
	code, err := netauth.Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"password": {exposedPassword}, "code": {code}, "next": {"/"}}
	req, _ := http.NewRequest(http.MethodPost, addr+"/__auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	login, err := c.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", login.StatusCode, http.StatusSeeOther)
	}

	// And now the API answers over the exposed address.
	if resp := fetch(t, c, addr+"/api/projects", "*/*"); resp.StatusCode != http.StatusOK {
		t.Errorf("api status after login = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestTheWindowNeverSignsIn is the property the whole design rests on: the
// lock is on the extra listener, so the desktop window's own connection is
// unaffected and a forgotten password can never shut anybody out of Settings.
func TestTheWindowNeverSignsIn(t *testing.T) {
	s, addr := exposed(t)
	lockTheServer(t, s)

	if resp := do(t, s, "GET", "/api/projects", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the window's own request = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	// Same request, same server, through the exposed listener instead.
	if resp := fetch(t, browser(), addr+"/api/projects", "*/*"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the browser's request = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestLockTurnsOffAgain checks the switch works both ways, and that turning
// it off really does open the exposed listener rather than only saying so.
func TestLockTurnsOffAgain(t *testing.T) {
	s, addr := exposed(t)
	lockTheServer(t, s)

	resp := do(t, s, "PUT", "/api/server/auth", map[string]any{"enabled": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: status %d", resp.StatusCode)
	}
	if info := decode[serverConfigInfo](t, resp); info.AuthEnabled || !info.HasPassword {
		t.Errorf("got %+v, want the lock off but the password still on file", info)
	}
	if resp := fetch(t, browser(), addr+"/api/projects", "*/*"); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d once the lock is off", resp.StatusCode, http.StatusOK)
	}
}

func TestLockWithoutPasswordBlocksAccess(t *testing.T) {
	s, addr := exposed(t)
	resp := do(t, s, "PUT", "/api/server/auth", map[string]any{"enabled": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	info := decode[serverConfigInfo](t, resp)
	if !info.AuthEnabled || info.HasPassword || info.TOTPSecret == "" {
		t.Fatalf("want enabled lock with generated seed and no password: %+v", info)
	}
	saved := decode[serverConfigInfo](t, do(t, s, "GET", "/api/server", nil))
	if !saved.AuthEnabled || saved.TOTPSecret != info.TOTPSecret {
		t.Fatal("lock was not persisted")
	}
	client := browser()
	for _, path := range []string{"/api/server", "/api/server/auth/code"} {
		r := fetch(t, client, addr+path, "application/json")
		r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d", path, r.StatusCode)
		}
	}
	code, _ := netauth.Code(info.TOTPSecret, time.Now())
	r, err := client.PostForm(addr+"/__auth/login", url.Values{"password": {""}, "code": {code}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty password login: %d", r.StatusCode)
	}
}

func TestServerCurrentCode(t *testing.T) {
	s, _ := exposed(t)
	missing := do(t, s, "GET", "/api/server/auth/code", nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("no seed: %d", missing.StatusCode)
	}
	seed := lockTheServer(t, s)
	before := time.Now()
	resp := do(t, s, "GET", "/api/server/auth/code", nil)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("code must not be cached")
	}
	var result struct {
		Code           string `json:"code"`
		RefreshAfterMS int64  `json:"refresh_after_ms"`
	}
	result = decode[struct {
		Code           string `json:"code"`
		RefreshAfterMS int64  `json:"refresh_after_ms"`
	}](t, resp)
	first, _ := netauth.Code(seed, before)
	last, _ := netauth.Code(seed, time.Now())
	if result.Code != first && result.Code != last {
		t.Fatalf("wrong current code: %q", result.Code)
	}
	if result.RefreshAfterMS < 0 || result.RefreshAfterMS > 30000 {
		t.Fatalf("invalid refresh delay: %d", result.RefreshAfterMS)
	}
}

// TestShortPasswordRejected checks the one rule there is about passwords, and
// that a rejected one changes nothing.
func TestShortPasswordRejected(t *testing.T) {
	s, _ := exposed(t)
	resp := do(t, s, "PUT", "/api/server/auth", map[string]any{"enabled": true, "password": "short"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if info := decode[serverConfigInfo](t, do(t, s, "GET", "/api/server", nil)); info.HasPassword {
		t.Error("a rejected password was saved anyway")
	}
}

// TestSeedCanBeSetAndRolled is the pair of things the pane offers beside the
// seed: type in one you already have, or take a fresh random one.
func TestSeedCanBeSetAndRolled(t *testing.T) {
	s, _ := exposed(t)
	first := lockTheServer(t, s)

	// Typed in, in the shape a person would paste it.
	resp := do(t, s, "PUT", "/api/server/auth", map[string]any{
		"enabled": true, "totp_secret": "gezd gnbv gy3t qojq",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set seed: status %d", resp.StatusCode)
	}
	if got := decode[serverConfigInfo](t, resp).TOTPSecret; got != "GEZDGNBVGY3TQOJQ" {
		t.Errorf("seed = %q, want it normalised", got)
	}

	// A seed that is not one is refused, and the good one stays.
	if resp := do(t, s, "PUT", "/api/server/auth", map[string]any{
		"enabled": true, "totp_secret": "nonsense!",
	}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad seed: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Rolled fresh.
	rolled := decode[serverConfigInfo](t, do(t, s, "POST", "/api/server/auth/totp", nil))
	if rolled.TOTPSecret == "GEZDGNBVGY3TQOJQ" || rolled.TOTPSecret == first {
		t.Errorf("rolled seed = %q, want a new one", rolled.TOTPSecret)
	}
	if !rolled.AuthEnabled {
		t.Error("rolling a seed turned the lock off")
	}
}

// TestQRDrawsTheCurrentSeed checks the picture the pane shows is a PNG, and
// that there is none to show before a seed exists.
func TestQRDrawsTheCurrentSeed(t *testing.T) {
	s, _ := exposed(t)
	if resp := do(t, s, "GET", "/api/server/auth/totp.png", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status before a seed = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	lockTheServer(t, s)
	resp := do(t, s, "GET", "/api/server/auth/totp.png", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "png") {
		t.Errorf("Content-Type = %q, want a PNG", ct)
	}
	if body := read(t, resp); !strings.HasPrefix(body, "\x89PNG") {
		t.Errorf("body is not a PNG: %q", body[:min(8, len(body))])
	}
}

// TestMovingTheServerKeepsTheLock guards the two writers sharing one row, at
// the level the UI actually drives them: changing the port must not be a way
// to take the lock off.
func TestMovingTheServerKeepsTheLock(t *testing.T) {
	s, _ := exposed(t)
	lockTheServer(t, s)

	resp := do(t, s, "PUT", "/api/server", map[string]any{
		"enabled": false, "host": "127.0.0.1", "port": 7999,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("move: status %d", resp.StatusCode)
	}
	info := decode[serverConfigInfo](t, resp)
	if !info.AuthEnabled || !info.HasPassword || info.TOTPSecret == "" {
		t.Errorf("got %+v, want the lock untouched by a move", info)
	}
}

func read(t *testing.T, resp *http.Response) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.String()
}
