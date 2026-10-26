// Package netserver is the desktop shell's optional extra listener: turned
// on, it reverse-proxies a chosen host and port to the same loopback address
// the window's own view already talks to, so agenttik is reachable from a
// browser too — this machine or another on the network — without touching
// the window's own connection. Off, or web mode, and none of this runs; the
// window's loopback proxy in app/cmd/agenttik is untouched either way.
package netserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// shutdownTimeout bounds how long Stop, and a replaced listener in Start,
// wait for what is in flight — including an open SSE stream — before cutting
// it off. Matches the server package's own shutdown cap.
const shutdownTimeout = 3 * time.Second

// Manager owns at most one extra listener at a time. Starting a second one
// replaces the first once the new one is actually up, which is how a host or
// a port change is applied without a gap — except on the same port, see Start.
type Manager struct {
	target string // loopback address every request is proxied to
	// wrap goes in front of the proxy on every listener this manager opens:
	// the exposed server's optional lock, see app/internal/netauth. Nil
	// means there is nothing in front of it.
	wrap func(http.Handler) http.Handler

	mu   sync.Mutex
	srv  *http.Server
	ln   net.Listener // srv's socket, closed early by a same-port move
	addr string       // what is actually bound right now, empty while stopped
	err  error        // why the last Start failed, if it did
}

// New returns a manager that proxies to target, the loopback address the
// desktop window's own view is already served from.
func New(target string) *Manager { return &Manager{target: target} }

// Target is the loopback address every request is proxied to.
func (m *Manager) Target() string { return m.target }

// Use puts h in front of the proxy, for every listener this manager opens
// from here on. Call it during wiring, before Start; nothing guards the
// field afterwards.
func (m *Manager) Use(wrap func(http.Handler) http.Handler) { m.wrap = wrap }

// Status is the manager's actual state, as opposed to what was last asked
// for — the two can disagree when a saved setting fails to bind again on
// startup, e.g. because something else now holds that port.
type Status struct {
	Listening bool
	Addr      string
	Error     string
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Listening: m.srv != nil, Addr: m.addr}
	if m.err != nil {
		st.Error = m.err.Error()
	}
	return st
}

// Start listens on addr ("host:port") and proxies every request to the
// target given to New. On success it replaces whatever this manager was
// already listening on; on failure the previous listener, if any, is left
// running as it was.
func (m *Manager) Start(addr string) error {
	target, err := url.Parse("http://" + m.target)
	if err != nil {
		return err
	}

	m.mu.Lock()
	old, oldLn, oldAddr := m.srv, m.ln, m.addr
	m.mu.Unlock()

	// The kernel will not bind a port this manager already listens on, not
	// even on another host: 0.0.0.0 overlaps 127.0.0.1. So a same-port move
	// lets go of the old socket first. The old server keeps answering what is
	// already open meanwhile, and gets its socket back if the bind fails.
	samePort := old != nil && port(addr) != "0" && port(addr) == port(oldAddr)
	if samePort {
		oldLn.Close()
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if samePort {
			m.reopen(old, oldAddr)
		}
		m.mu.Lock()
		m.err = err
		m.mu.Unlock()
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1 // flush at once, so SSE still streams
	var handler http.Handler = proxy
	if m.wrap != nil {
		handler = m.wrap(proxy)
	}
	srv := &http.Server{Handler: handler}

	m.mu.Lock()
	m.srv, m.ln, m.addr, m.err = srv, ln, ln.Addr().String(), nil
	m.mu.Unlock()

	go srv.Serve(ln) //nolint:errcheck // Shutdown's own ErrServerClosed; nothing to report

	if old != nil {
		shutdown(old)
	}
	return nil
}

// reopen puts srv back on addr after a same-port move failed to bind. If even
// that fails, srv is shut down and the manager reads as stopped.
func (m *Manager) reopen(srv *http.Server, addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		shutdown(srv)
		m.mu.Lock()
		m.srv, m.ln, m.addr = nil, nil, ""
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	m.ln = ln
	m.mu.Unlock()
	go srv.Serve(ln) //nolint:errcheck // as in Start
}

// Stop closes whatever this manager is listening on. Safe to call when it
// already is not.
func (m *Manager) Stop() error {
	m.mu.Lock()
	srv := m.srv
	m.srv, m.ln, m.addr, m.err = nil, nil, "", nil
	m.mu.Unlock()
	if srv == nil {
		return nil
	}
	return shutdown(srv)
}

// shutdown stops srv, giving what is in flight up to shutdownTimeout.
func shutdown(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// port is the port part of a "host:port" address, empty if it has none.
func port(addr string) string {
	_, p, _ := net.SplitHostPort(addr)
	return p
}
