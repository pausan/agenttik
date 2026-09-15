package netauth

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/remote"
	"golang.org/x/crypto/bcrypt"
)

const (
	// cookieName holds the browser's session token. The token is random and
	// kept only in this process's memory, so closing agenttik ends every
	// browser session with it — there is no signing key on disk to steal and
	// nothing to revoke after the fact.
	cookieName = "agenttik_session"

	// idleTimeout is how long a browser may go quiet before it has to log in
	// again. Every request it makes pushes the deadline back.
	idleTimeout = 12 * time.Hour

	// maxFailures failed attempts from one address within failWindow lock
	// that address out for the rest of the window. bcrypt already puts about
	// a tenth of a second under every guess; this is what stops a patient
	// script rather than a fast one.
	maxFailures = 5
	failWindow  = 15 * time.Minute

	// prefix namespaces the two paths the gate answers itself. Everything
	// else it either proxies or refuses, so these must be paths the UI's own
	// router will never want.
	prefix     = "/__auth/"
	loginPath  = prefix + "login"
	logoutPath = prefix + "logout"
)

// Credentials is what a login is checked against. The gate reads them fresh
// on every attempt rather than holding a copy, so a password changed in
// Settings is in force for the very next request.
type Credentials struct {
	Enabled      bool
	PasswordHash string
	Secret       string
}

// ready reports whether these credentials can actually let anybody in. A
// half-set lock — enabled with no password, or none of a secret — would
// otherwise be a lock nobody holds the key to, so the gate refuses every
// login instead of opening.
func (c Credentials) ready() bool {
	return c.Enabled && c.PasswordHash != "" && c.Secret != ""
}

// Gate is the lock in front of the exposed listener. Its zero value is not
// usable; call New.
type Gate struct {
	read func() Credentials

	mu       sync.Mutex
	sessions map[string]time.Time // token -> when it goes stale
	failures map[string]*failure  // client address -> recent bad attempts
	usedStep uint64               // the last TOTP step accepted, never accepted twice
}

type failure struct {
	count int
	last  time.Time
}

// New returns a gate that checks every login against whatever read answers at
// that moment.
func New(read func() Credentials) *Gate {
	return &Gate{
		read:     read,
		sessions: map[string]time.Time{},
		failures: map[string]*failure{},
	}
}

// Wrap puts the gate in front of next. With the lock turned off in Settings
// it hands every request straight through, which is what makes turning it on
// and off take effect without rebinding the listener.
func (g *Gate) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == remote.VersionPath {
			next.ServeHTTP(w, r)
			return
		}
		creds := g.read()
		if !creds.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case loginPath:
			g.login(w, r, creds)
			return
		case logoutPath:
			g.logout(w, r)
			return
		}
		if g.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		g.challenge(w, r, creds, "", r.URL.RequestURI())
	})
}

// Revoke ends every open browser session. Changing the password or the seed
// calls it: whoever was already logged in got in with a secret that no longer
// exists, and the point of changing one is usually that somebody else knows
// it.
func (g *Gate) Revoke() {
	g.mu.Lock()
	defer g.mu.Unlock()
	clear(g.sessions)
}

// authorized reports whether the request carries a live session, and pushes
// that session's idle deadline back if it does.
func (g *Gate) authorized(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	deadline, ok := g.sessions[c.Value]
	if !ok {
		return false
	}
	if now.After(deadline) {
		delete(g.sessions, c.Value)
		return false
	}
	g.sessions[c.Value] = now.Add(idleTimeout)
	return true
}

// login checks a password and a code, and starts a session if both hold. It
// answers the form it was posted from, so a failure comes back as the same
// page with a reason on it rather than as a bare status.
func (g *Gate) login(w http.ResponseWriter, r *http.Request, creds Credentials) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		g.challenge(w, r, creds, "That form could not be read.", "/")
		return
	}
	next := safeNext(r.FormValue("next"))

	if locked, until := g.lockedOut(clientAddr(r)); locked {
		g.deny(w, r, creds, "Too many attempts. Try again in "+humanWait(until)+".")
		return
	}
	// A lock nobody can open is not a reason to let anybody in: an enabled
	// setting missing its password or its seed refuses every login, and the
	// window's own Settings is where that gets put right.
	if !creds.ready() {
		g.deny(w, r, creds, "Sign-in is not finished being set up on this machine.")
		return
	}

	// Both are checked every time, even when the first already failed, so a
	// wrong password and a wrong code cost the same and take the same time to
	// say so.
	passOK := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(r.FormValue("password"))) == nil
	step, codeOK := Verify(creds.Secret, r.FormValue("code"), time.Now())
	if !passOK || !codeOK || !g.claimStep(step) {
		g.deny(w, r, creds, "Wrong password or code.")
		return
	}

	token, err := newToken()
	if err != nil {
		http.Error(w, "could not start a session", http.StatusInternalServerError)
		return
	}
	g.mu.Lock()
	g.sessions[token] = time.Now().Add(idleTimeout)
	delete(g.failures, clientAddr(r))
	g.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(idleTimeout / time.Second),
	})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// logout drops this browser's session and sends it back to the form.
func (g *Gate) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		g.mu.Lock()
		delete(g.sessions, c.Value)
		g.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// claimStep accepts a TOTP step once and once only. The same six digits are
// good for a minute and a half either side of their own, which is a long
// while for someone who read them off a screen to type them in as well.
func (g *Gate) claimStep(step uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if step <= g.usedStep {
		return false
	}
	g.usedStep = step
	return true
}

// deny records a failed attempt against the client's address and answers with
// the form again. Recording happens here rather than in login so that every
// way of getting it wrong counts the same.
func (g *Gate) deny(w http.ResponseWriter, r *http.Request, creds Credentials, why string) {
	addr := clientAddr(r)
	now := time.Now()
	g.mu.Lock()
	f := g.failures[addr]
	if f == nil || now.Sub(f.last) > failWindow {
		f = &failure{}
		g.failures[addr] = f
	}
	f.count++
	f.last = now
	// The map only ever holds addresses that got something wrong recently, so
	// sweeping it here keeps it from growing for the life of the process.
	for k, v := range g.failures {
		if now.Sub(v.last) > failWindow {
			delete(g.failures, k)
		}
	}
	g.mu.Unlock()
	g.challenge(w, r, creds, why, r.FormValue("next"))
}

// lockedOut reports whether an address has used up its attempts, and when it
// gets them back.
func (g *Gate) lockedOut(addr string) (bool, time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	f := g.failures[addr]
	if f == nil || f.count < maxFailures || time.Since(f.last) > failWindow {
		return false, time.Time{}
	}
	return true, f.last.Add(failWindow)
}

// challenge answers a request that has no session. A browser asking for a
// page gets the form; anything else — the UI's own fetches, an open event
// stream, curl — gets a status it can act on, because answering those with
// HTML would only put a login page where the UI expects JSON. next is where
// a good login should land: the page that was asked for, or the one the form
// was already carrying.
func (g *Gate) challenge(w http.ResponseWriter, r *http.Request, creds Credentials, why, next string) {
	form := wantsHTML(r) && (r.Method == http.MethodGet || r.URL.Path == loginPath)
	if !form {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"sign in to reach this server"}`)) //nolint:errcheck // client is gone
		return
	}
	status := http.StatusOK
	if why != "" {
		status = http.StatusUnauthorized
	}
	writeLoginPage(w, status, why, safeNext(next), creds.ready())
}

// wantsHTML reports whether this request came from a browser's address bar
// rather than from the UI's own code. fetch() sends Accept: */*, and the SSE
// stream asks for text/event-stream; only a navigation asks for HTML first.
func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// safeNext keeps only a path on this same server, so the form cannot be
// handed a target that sends the browser somewhere else after a good login.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

// clientAddr is who the attempt came from, for counting failures. The
// listener is reached directly rather than through a proxy of the user's, so
// the connection's own address is the honest answer and a forwarding header
// would only be a way to get somebody else locked out.
func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// humanWait rounds a deadline to something worth reading on a login form.
func humanWait(until time.Time) string {
	d := time.Until(until).Round(time.Minute)
	if d < time.Minute {
		return "a minute"
	}
	return d.String()
}
