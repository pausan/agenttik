package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"maps"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/remote"
)

// savedRemote is another machine whose profiles this instance lists. It is
// keyed by the remote's machine ID, so a new address updates the same entry.
// Cookies hold the remote's sign-in session; they never reach the browser.
type savedRemote struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Version  string            `json:"version,omitempty"`
	Profiles []Profile         `json:"profiles"`
	Cookies  map[string]string `json:"cookies,omitempty"`
}

// remoteView is what the UI sees of a saved remote.
type remoteView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Version  string    `json:"version,omitempty"`
	Profiles []Profile `json:"profiles"`
	SignedIn bool      `json:"signed_in"`
}

type remoteScan struct {
	id      string
	cancel  context.CancelFunc
	probed  atomic.Uint64
	total   uint64
	running bool
	found   string
	err     string
}

type remoteManager struct {
	path     string
	self     func() (string, error)
	networks func() ([]netip.Prefix, error)
	client   *http.Client

	mu      sync.Mutex
	loaded  bool
	remotes []savedRemote
	scan    *remoteScan
}

func newRemoteManager(dir string, self func() (string, error)) *remoteManager {
	return &remoteManager{
		path:     filepath.Join(dir, "remotes.json"),
		self:     self,
		networks: remote.LocalNetworks,
		// No overall timeout: event streams and videos stay open. Redirects
		// are answered to the UI rather than followed with its cookies.
		client: &http.Client{
			Transport:     http.DefaultTransport.(*http.Transport).Clone(),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// load reads the catalog on first use. Startup never contacts a remote.
func (m *remoteManager) load() error {
	if m.loaded {
		return nil
	}
	data, err := os.ReadFile(m.path)
	if err == nil {
		if err := json.Unmarshal(data, &m.remotes); err != nil {
			return errors.New("read remotes: " + err.Error())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m.loaded = true
	return nil
}

func (m *remoteManager) save(remotes []savedRemote) error {
	data, err := json.Marshal(remotes)
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return err
	}
	m.remotes = remotes
	return nil
}

// update applies fn to a copy of the remote and saves it.
func (m *remoteManager) update(id string, fn func(*savedRemote)) (savedRemote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return savedRemote{}, err
	}
	next := append([]savedRemote{}, m.remotes...)
	for i := range next {
		if next[i].ID == id {
			r := next[i]
			r.Cookies = maps.Clone(r.Cookies)
			if r.Cookies == nil {
				r.Cookies = map[string]string{}
			}
			fn(&r)
			next[i] = r
			return r, m.save(next)
		}
	}
	return savedRemote{}, fiber.NewError(404, "Remote machine not found")
}

func (m *remoteManager) get(id string) (savedRemote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return savedRemote{}, err
	}
	for _, r := range m.remotes {
		if r.ID == id {
			r.Cookies = maps.Clone(r.Cookies)
			return r, nil
		}
	}
	return savedRemote{}, fiber.NewError(404, "Remote machine not found")
}

func (r savedRemote) view() remoteView {
	profiles := r.Profiles
	if profiles == nil {
		profiles = []Profile{}
	}
	return remoteView{ID: r.ID, Name: r.Name, Address: r.Address, Version: r.Version, Profiles: profiles, SignedIn: len(r.Cookies) > 0}
}

// request calls the remote with its saved session. Cookies it sets are kept.
func (m *remoteManager) request(ctx context.Context, r savedRemote, method, path string, body io.Reader, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, r.Address+path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	for name, value := range r.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	if cookies := res.Cookies(); len(cookies) > 0 {
		m.update(r.ID, func(saved *savedRemote) {
			for _, c := range cookies {
				if c.MaxAge < 0 || c.Value == "" {
					delete(saved.Cookies, c.Name)
				} else {
					saved.Cookies[c.Name] = c.Value
				}
			}
		})
	}
	return res, nil
}

func (s *Server) remoteManager() (*remoteManager, error) {
	if s.remotes == nil {
		return nil, fiber.NewError(503, "Remote profiles are unavailable")
	}
	return s.remotes, nil
}

func (s *Server) listRemotes(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return err
	}
	views := make([]remoteView, 0, len(m.remotes))
	for _, r := range m.remotes {
		views = append(views, r.view())
	}
	return c.JSON(fiber.Map{"remotes": views})
}

// addRemote checks an address and saves the machine that answers there. A
// machine already saved keeps its session and moves to the new address.
func (s *Server) addRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	var body struct {
		Address string `json:"address"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid remote")
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	target, info, err := remote.Check(ctx, body.Address)
	if err != nil {
		return badRequest("%s", err.Error())
	}
	if info.ID == "" {
		return badRequest("that agenttik is too old to be added; update it first")
	}
	if self, err := m.self(); err == nil && self == info.ID {
		return badRequest("that address is this agenttik")
	}
	m.mu.Lock()
	if err := m.load(); err != nil {
		m.mu.Unlock()
		return err
	}
	next := append([]savedRemote{}, m.remotes...)
	found := false
	for i := range next {
		if next[i].ID == info.ID {
			next[i].Address, next[i].Name, next[i].Version = target.String(), remoteName(info, target.String()), info.Version
			found = true
		}
	}
	if !found {
		next = append(next, savedRemote{ID: info.ID, Name: remoteName(info, target.String()), Address: target.String(), Version: info.Version})
	}
	err = m.save(next)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return c.Status(201).JSON(m.connect(c.Context(), info.ID))
}

// remoteName falls back to the address for servers that have no name.
func remoteName(info remote.Info, address string) string {
	if info.Name != "" {
		return info.Name
	}
	if u, err := url.Parse(address); err == nil && u.Host != "" {
		return u.Host
	}
	return address
}

type remoteState struct {
	// Status is ok, signin, unreachable or moved. Moved means another machine
	// answers at the saved address.
	Status       string      `json:"status"`
	Error        string      `json:"error,omitempty"`
	CodeRequired bool        `json:"code_required,omitempty"`
	Remote       *remoteView `json:"remote,omitempty"`
	Busy         []string    `json:"busy,omitempty"`
}

func (s *Server) connectRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	if _, err := m.get(c.Params("id")); err != nil {
		return err
	}
	return c.JSON(m.connect(c.Context(), c.Params("id")))
}

// connect checks that the saved address still reaches the same machine and
// refreshes its profile list. Only an explicit switch or check calls it.
func (m *remoteManager) connect(parent context.Context, id string) remoteState {
	r, err := m.get(id)
	if err != nil {
		return remoteState{Status: "unreachable", Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	_, info, err := remote.Check(ctx, r.Address)
	if err != nil {
		view := r.view()
		return remoteState{Status: "unreachable", Error: err.Error(), Remote: &view}
	}
	if info.ID != r.ID {
		view := r.view()
		return remoteState{Status: "moved", Error: "another agenttik answers at " + r.Address, Remote: &view}
	}
	res, err := m.request(ctx, r, "GET", "/api/profiles", nil, http.Header{"Accept": {"application/json"}})
	if err != nil {
		view := r.view()
		return remoteState{Status: "unreachable", Error: err.Error(), Remote: &view}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		r, _ = m.update(id, func(saved *savedRemote) {
			saved.Name, saved.Version, saved.Cookies = remoteName(info, r.Address), info.Version, map[string]string{}
		})
		view := r.view()
		return remoteState{Status: "signin", Remote: &view, CodeRequired: m.codeRequired(ctx, r)}
	}
	var list struct {
		Profiles []struct {
			Profile
			Busy bool `json:"busy"`
		} `json:"profiles"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&list) != nil {
		view := r.view()
		return remoteState{Status: "unreachable", Error: "the remote did not list its profiles", Remote: &view}
	}
	profiles := make([]Profile, 0, len(list.Profiles))
	var busy []string
	for _, p := range list.Profiles {
		profiles = append(profiles, p.Profile)
		if p.Busy {
			busy = append(busy, p.ID)
		}
	}
	r, err = m.update(id, func(saved *savedRemote) {
		saved.Name, saved.Version, saved.Profiles = remoteName(info, r.Address), info.Version, profiles
	})
	if err != nil {
		return remoteState{Status: "unreachable", Error: err.Error()}
	}
	view := r.view()
	return remoteState{Status: "ok", Remote: &view, Busy: busy}
}

// codeRequired reads the remote's sign-in form: it asks for a code only when
// two-factor sign-in is on.
func (m *remoteManager) codeRequired(ctx context.Context, r savedRemote) bool {
	res, err := m.request(ctx, r, "GET", "/", nil, http.Header{"Accept": {"text/html"}})
	if err != nil {
		return true
	}
	defer res.Body.Close()
	page, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	return bytes.Contains(page, []byte(`name="code"`))
}

var loginReason = regexp.MustCompile(`<p class="bad">([^<]*)</p>`)

// signInRemote posts the remote's own sign-in form and keeps the session.
func (s *Server) signInRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	r, err := m.get(c.Params("id"))
	if err != nil {
		return err
	}
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid sign-in")
	}
	form := url.Values{"password": {body.Password}, "code": {body.Code}, "next": {"/"}}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	res, err := m.request(ctx, r, "POST", "/__auth/login", strings.NewReader(form.Encode()),
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"text/html"}})
	if err != nil {
		return fiber.NewError(502, "Could not reach the remote machine")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		page, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		reason := "Sign-in failed"
		if match := loginReason.FindSubmatch(page); match != nil {
			reason = html.UnescapeString(string(match[1]))
		}
		return fiber.NewError(401, reason)
	}
	return c.JSON(m.connect(c.Context(), r.ID))
}

func (s *Server) deleteRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	id := c.Params("id")
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return err
	}
	next := make([]savedRemote, 0, len(m.remotes))
	for _, r := range m.remotes {
		if r.ID != id {
			next = append(next, r)
		}
	}
	if len(next) == len(m.remotes) {
		return fiber.NewError(404, "Remote machine not found")
	}
	if m.scan != nil && m.scan.id == id {
		m.scan.cancel()
	}
	if err := m.save(next); err != nil {
		return err
	}
	return c.SendStatus(204)
}

var errRemoteFound = errors.New("found")

// findRemote scans the local private networks for the saved machine ID, on
// the port of its last address. Starting another scan stops the previous one.
func (s *Server) findRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	r, err := m.get(c.Params("id"))
	if err != nil {
		return err
	}
	u, err := url.Parse(r.Address)
	if err != nil {
		return err
	}
	if u.Scheme != "http" {
		return badRequest("only HTTP servers can be found on the network")
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	networks, err := m.networks()
	if err != nil {
		return err
	}
	if len(networks) == 0 {
		return badRequest("no active private IPv4 networks found")
	}
	ctx, cancel := context.WithCancel(context.Background())
	scan := &remoteScan{id: r.ID, cancel: cancel, total: remote.Hosts(networks), running: true}
	m.mu.Lock()
	if m.scan != nil {
		m.scan.cancel()
	}
	m.scan = scan
	m.mu.Unlock()
	go func() {
		err := remote.Scan(ctx, networks, port, &scan.probed, func(target *url.URL, info remote.Info) error {
			if info.ID != r.ID {
				return nil
			}
			if _, err := m.update(r.ID, func(saved *savedRemote) {
				saved.Address, saved.Name, saved.Version = target.String(), remoteName(info, target.String()), info.Version
			}); err != nil {
				return err
			}
			m.mu.Lock()
			scan.found = target.String()
			m.mu.Unlock()
			return errRemoteFound
		})
		m.mu.Lock()
		scan.running = false
		if err != nil && !errors.Is(err, errRemoteFound) && !errors.Is(err, context.Canceled) {
			scan.err = err.Error()
		}
		m.mu.Unlock()
		cancel()
	}()
	return c.Status(202).JSON(m.scanState(r.ID))
}

func (m *remoteManager) scanState(id string) fiber.Map {
	m.mu.Lock()
	defer m.mu.Unlock()
	scan := m.scan
	if scan == nil || scan.id != id {
		return fiber.Map{"running": false}
	}
	return fiber.Map{"running": scan.running, "probed": scan.probed.Load(), "total": scan.total, "found": scan.found, "error": scan.err}
}

func (s *Server) findRemoteStatus(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	return c.JSON(m.scanState(c.Params("id")))
}

func (s *Server) stopFindRemote(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.scan != nil && m.scan.id == c.Params("id") {
		m.scan.cancel()
	}
	m.mu.Unlock()
	return c.SendStatus(204)
}

func (m *remoteManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scan != nil {
		m.scan.cancel()
	}
}

// Paths that belong to this window's own instance even while it shows a
// remote profile: the profile and remote catalogs, the window, and updates.
func remoteStaysLocal(path string) bool {
	for _, prefix := range []string{"/api/remotes", "/api/remote/", "/api/profiles", "/api/updates", "/api/desktop", "/api/foreground"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix)) {
			return true
		}
	}
	return false
}

// Request headers worth passing on. Everything else, including cookies, the
// update token and the browser's origin, stays on this machine.
var forwardedHeaders = []string{"Accept", "Cache-Control", "Content-Type", "If-Modified-Since", "If-None-Match", "If-Range", "Last-Event-ID", "Range"}

// routeRemote sends an API request that names a remote to that machine,
// carrying this instance's session for it. The remote sees the profile in the
// query as one of its own.
func (s *Server) routeRemote(c *fiber.Ctx) error {
	id := c.Query("remote")
	if id == "" || s.remotes == nil || !strings.HasPrefix(c.Path(), "/api/") || remoteStaysLocal(c.Path()) {
		return c.Next()
	}
	m := s.remotes
	r, err := m.get(id)
	if err != nil {
		return err
	}
	args := c.Request().URI().QueryArgs()
	query := url.Values{}
	args.VisitAll(func(k, v []byte) {
		if string(k) != "remote" {
			query.Add(string(k), string(v))
		}
	})
	path := c.Path()
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	header := http.Header{}
	for _, name := range forwardedHeaders {
		if v := c.Get(name); v != "" {
			header.Set(name, v)
		}
	}
	var body io.Reader
	if b := c.Body(); len(b) > 0 {
		body = bytes.NewReader(b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	res, err := m.request(ctx, r, c.Method(), path, body, header)
	if err != nil {
		cancel()
		return fiber.NewError(502, "Could not reach "+r.Name)
	}
	for name, values := range res.Header {
		switch http.CanonicalHeaderKey(name) {
		case "Set-Cookie", "Connection", "Transfer-Encoding", "Content-Length", "Content-Encoding", "X-Agenttik-Instance", "Location":
			continue
		}
		for _, v := range values {
			c.Response().Header.Add(name, v)
		}
	}
	c.Set("X-Agenttik-Remote", "machine:"+r.ID)
	if res.StatusCode == http.StatusUnauthorized {
		c.Set("X-Agenttik-Remote-Auth", "required")
	}
	c.Status(res.StatusCode)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		c.Response().SetBodyStream(cancelOnClose{res.Body, cancel}, int(res.ContentLength))
		return nil
	}
	// A closed window is noticed at the next heartbeat; shutdown is at once.
	go func() {
		select {
		case <-s.closing:
			cancel()
		case <-ctx.Done():
		}
	}()
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer cancel()
		defer res.Body.Close()
		buf := make([]byte, 32<<10)
		for {
			n, err := res.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil || w.Flush() != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	})
	return nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}
