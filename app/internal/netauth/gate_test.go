package netauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const testPassword = "correct horse battery"

// testCreds is a lock that is fully set up. The hash is made at bcrypt's
// minimum cost: the default is a tenth of a second per guess on purpose, and
// these tests make a lot of guesses.
func testCreds() Credentials {
	h, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return Credentials{Enabled: true, PasswordHash: string(h), Secret: rfcSecret}
}

// gated returns a gate over a handler that says "behind", so a test can tell
// what got through from what the gate answered itself.
func gated(creds Credentials) (*Gate, http.Handler) {
	g := New(func() Credentials { return creds })
	return g, g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("behind")) //nolint:errcheck // test handler
	}))
}

// get makes the request a browser's address bar would.
func get(h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	r.RemoteAddr = "10.0.0.9:51000"
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// attempt posts the login form the way the page does.
func attempt(h http.Handler, password, code string, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	form := url.Values{"password": {password}, "code": {code}}
	r := httptest.NewRequest(http.MethodPost, loginPath, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "text/html")
	r.RemoteAddr = "10.0.0.9:51000"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range (&http.Response{Header: w.Header()}).Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie was set (status %d)", cookieName, w.Code)
	return nil
}

func liveCode(t *testing.T) string {
	t.Helper()
	code, err := Code(rfcSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// TestDisabledPassesEverythingThrough is the setting that ships: the exposed
// server behaves exactly as it did before there was a lock at all.
func TestDisabledPassesEverythingThrough(t *testing.T) {
	_, h := gated(Credentials{Enabled: false})
	if w := get(h, "/"); w.Body.String() != "behind" {
		t.Errorf("body = %q, want the handler behind the gate", w.Body.String())
	}
	// Even the gate's own paths belong to the app when the lock is off.
	if w := get(h, loginPath); w.Body.String() != "behind" {
		t.Errorf("%s = %q, want the handler behind the gate", loginPath, w.Body.String())
	}
}

// TestBrowserGetsTheForm checks that a navigation is answered with a page to
// log in on, and that nothing of the app comes with it.
func TestBrowserGetsTheForm(t *testing.T) {
	_, h := gated(testCreds())
	w := get(h, "/")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, `name="password"`) || strings.Contains(body, "behind") {
		t.Errorf("body = %q, want the login form and nothing from behind the gate", body)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// TestNonBrowserGetsJSON checks that the UI's own fetches and the event
// stream are refused with a status rather than handed an HTML page where
// they expect data.
func TestNonBrowserGetsJSON(t *testing.T) {
	_, h := gated(testCreds())
	for name, accept := range map[string]string{
		"fetch":  "*/*",
		"stream": "text/event-stream",
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want %d", name, w.Code, http.StatusUnauthorized)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type = %q, want application/json", name, ct)
		}
	}
}

// TestGoodLoginOpensTheGate walks the whole way through: form, cookie, app.
func TestGoodLoginOpensTheGate(t *testing.T) {
	_, h := gated(testCreds())
	w := attempt(h, testPassword, liveCode(t))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	c := sessionCookie(t, w)
	if !c.HttpOnly {
		t.Error("the session cookie is readable from script")
	}
	if got := get(h, "/", c).Body.String(); got != "behind" {
		t.Errorf("body = %q, want the handler behind the gate", got)
	}
}

// TestBadCredentialsStayOut checks each half of the lock on its own.
func TestBadCredentialsStayOut(t *testing.T) {
	for name, try := range map[string][2]string{
		"wrong password": {"not the password", ""},
		"wrong code":     {testPassword, "000000"},
		"neither":        {"nope", "123456"},
	} {
		_, h := gated(testCreds())
		password, code := try[0], try[1]
		if code == "" {
			code = liveCode(t)
		}
		w := attempt(h, password, code)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want %d", name, w.Code, http.StatusUnauthorized)
		}
		for _, c := range (&http.Response{Header: w.Header()}).Cookies() {
			if c.Name == cookieName && c.Value != "" {
				t.Errorf("%s: a session was started anyway", name)
			}
		}
	}
}

// TestCodeIsSpentOnce checks that reading six digits off somebody's screen is
// not enough to follow them in during the same half-minute.
func TestCodeIsSpentOnce(t *testing.T) {
	_, h := gated(testCreds())
	code := liveCode(t)
	if w := attempt(h, testPassword, code); w.Code != http.StatusSeeOther {
		t.Fatalf("first attempt: status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if w := attempt(h, testPassword, code); w.Code != http.StatusUnauthorized {
		t.Errorf("replay: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// TestLockoutAfterRepeatedFailures checks the throttle that makes guessing
// six digits impractical rather than merely slow.
func TestLockoutAfterRepeatedFailures(t *testing.T) {
	_, h := gated(testCreds())
	for i := 0; i < maxFailures; i++ {
		attempt(h, "wrong", "000000")
	}
	// Right password, right code, but this address has used up its turns.
	w := attempt(h, testPassword, liveCode(t))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "Too many attempts") {
		t.Errorf("body = %q, want the lockout message", w.Body.String())
	}
	// The lockout is per address, so another machine is unaffected.
	elsewhere := func(r *http.Request) { r.RemoteAddr = "10.0.0.10:51000" }
	if w := attempt(h, testPassword, liveCode(t), elsewhere); w.Code != http.StatusSeeOther {
		t.Errorf("another address: status = %d, want %d", w.Code, http.StatusSeeOther)
	}
}

// TestHalfSetLockOpensForNobody checks the case an interrupted setup leaves
// behind: asking for a password that was never set must not mean asking for
// nothing.
func TestHalfSetLockOpensForNobody(t *testing.T) {
	full := testCreds()
	for name, creds := range map[string]Credentials{
		"no password": {Enabled: true, Secret: rfcSecret},
		"no seed":     {Enabled: true, PasswordHash: full.PasswordHash},
	} {
		_, h := gated(creds)
		if got := get(h, "/").Body.String(); strings.Contains(got, "behind") {
			t.Errorf("%s: the app was served", name)
		}
		if w := attempt(h, testPassword, liveCode(t)); w.Code == http.StatusSeeOther {
			t.Errorf("%s: a login succeeded", name)
		}
	}
}

// TestRevokeEndsOpenSessions is what changing a password does to whoever was
// already signed in with the old one.
func TestRevokeEndsOpenSessions(t *testing.T) {
	g, h := gated(testCreds())
	c := sessionCookie(t, attempt(h, testPassword, liveCode(t)))
	if got := get(h, "/", c).Body.String(); got != "behind" {
		t.Fatalf("body = %q, want the handler behind the gate", got)
	}
	g.Revoke()
	if got := get(h, "/", c).Body.String(); strings.Contains(got, "behind") {
		t.Error("a revoked session still reached the app")
	}
}

// TestLogoutEndsThisSession checks the other half of that, from the browser's
// own side.
func TestLogoutEndsThisSession(t *testing.T) {
	_, h := gated(testCreds())
	c := sessionCookie(t, attempt(h, testPassword, liveCode(t)))

	r := httptest.NewRequest(http.MethodPost, logoutPath, nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := get(h, "/", c).Body.String(); strings.Contains(got, "behind") {
		t.Error("the session still reached the app after logging out")
	}
}

// TestNextStaysOnThisServer checks that the field carrying "where I was
// going" cannot carry "somewhere else entirely".
func TestNextStaysOnThisServer(t *testing.T) {
	for in, want := range map[string]string{
		"/projects/3":          "/projects/3",
		"//evil.example.com/":  "/",
		"https://evil.example": "/",
		"":                     "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}

	_, h := gated(testCreds())
	form := url.Values{
		"password": {testPassword},
		"code":     {liveCode(t)},
		"next":     {"//evil.example.com/"},
	}
	r := httptest.NewRequest(http.MethodPost, loginPath, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Location"); got != "/" {
		t.Errorf("Location = %q, want /", got)
	}
}

// TestFormRemembersWhereYouWereGoing checks the other side of that: a deep
// link asked for while signed out comes back on the form.
func TestFormRemembersWhereYouWereGoing(t *testing.T) {
	_, h := gated(testCreds())
	body := get(h, "/projects/3?tab=files").Body.String()
	if !strings.Contains(body, `value="/projects/3?tab=files"`) {
		t.Errorf("body = %q, want the asked-for path in the next field", body)
	}
}
