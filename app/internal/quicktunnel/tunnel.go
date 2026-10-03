package quicktunnel

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"
)

// The edge answers on port 7844 in two regions; alternating between them is
// what cloudflared's own edge discovery comes down to for one connection.
var edges = []string{"region1.v2.argotunnel.com:7844", "region2.v2.argotunnel.com:7844"}

// edgeServerName is the TLS name cloudflared uses for the HTTP/2 protocol.
const edgeServerName = "h2.cftunnel.com"

// The edge's certificate is issued by Cloudflare's Origin CA, which no system
// trust store carries. These are the roots cloudflared embeds for the same
// reason (tlsconfig/cloudflare_ca.go, Apache-2.0).
//
//go:embed cloudflare_ca.pem
var cloudflareCA []byte

var rootCAs = sync.OnceValue(func() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cloudflareCA)
	return pool
})

// Status is what the tunnel is doing right now.
type Status struct {
	Running   bool   // between Start and Stop
	URL       string // the public address, once one has been handed out
	Connected bool   // registered with the edge and taking requests
	Ready     bool   // URL resolves, so opening it now will not cache a miss
	Location  string // airport code of the Cloudflare data centre it landed in
	Error     string // why the last attempt failed, while it is not connected
}

// Client keeps at most one quick tunnel up, reconnecting it when the
// connection drops, and serves every request it brings to a local origin.
type Client struct {
	handler  http.Handler
	version  string
	clientID [16]byte

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	status Status
}

// New returns a client whose visitors reach target, the loopback address the
// desktop window's own view is served from, through wrap — the same lock the
// exposed listener is behind. version identifies agenttik to the edge.
func New(target string, wrap func(http.Handler) http.Handler, version string) *Client {
	var h http.Handler = newOrigin(target)
	if wrap != nil {
		h = wrap(h)
	}
	c := &Client{handler: h, version: version}
	rand.Read(c.clientID[:]) //nolint:errcheck // never fails
	return c
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Start brings the tunnel up in the background, replacing one already
// running. saved, when it holds a tunnel, is tried first so the address
// survives a restart; when the edge no longer knows it, a fresh tunnel — with
// a fresh address — is requested and handed to save.
func (c *Client) Start(saved Credentials, save func(Credentials)) {
	c.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.mu.Lock()
	c.cancel, c.done = cancel, done
	c.status = Status{Running: true}
	if saved.ID != "" {
		c.status.URL = saved.URL()
	}
	c.mu.Unlock()
	go func() {
		defer close(done)
		c.run(ctx, saved, save)
	}()
}

// Stop unregisters from the edge and closes the connection. Safe to call when
// nothing is running.
func (c *Client) Stop() {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.cancel, c.done = nil, nil
	c.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	c.mu.Lock()
	c.status = Status{}
	c.mu.Unlock()
}

func (c *Client) update(f func(*Status)) {
	c.mu.Lock()
	f(&c.status)
	c.mu.Unlock()
}

// run connects, and reconnects after a delay that doubles up to a minute,
// until ctx ends. A tunnel the edge refuses outright is replaced by a new one.
func (c *Client) run(ctx context.Context, creds Credentials, save func(Credentials)) {
	wait := time.Second
	var attempts uint8
	for i := 0; ctx.Err() == nil; i++ {
		var err error
		if creds.ID == "" {
			creds, err = Request(ctx, "agenttik/"+c.version)
			if err == nil {
				save(creds)
			}
		}
		connected := false
		if err == nil {
			c.update(func(s *Status) { s.URL = creds.URL() })
			err = c.connect(ctx, creds, edges[i%len(edges)], attempts, func(location string) {
				connected = true
				c.update(func(s *Status) { s.Connected, s.Location, s.Error = true, location, "" })
				go c.awaitPublished(ctx, creds.Hostname)
			})
		}
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errRejected) {
			creds = Credentials{}
		}
		if connected {
			wait, attempts = time.Second, 0
		} else if attempts < math.MaxUint8 {
			attempts++
		}
		c.update(func(s *Status) { s.Connected, s.Ready, s.Location, s.Error = false, false, "", err.Error() })
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, time.Minute)
	}
}

// awaitPublished marks the status ready once hostname resolves. A new
// tunnel's name takes a few seconds to appear, and a resolver asked before
// then remembers the miss for a minute or more, so the name is asked of
// trycloudflare.com's own nameservers, which no other lookup shares.
func (c *Client) awaitPublished(ctx context.Context, hostname string) {
	for ctx.Err() == nil {
		if published(ctx, hostname) {
			c.update(func(s *Status) {
				if s.Connected && s.URL == "https://"+hostname {
					s.Ready = true
				}
			})
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func published(ctx context.Context, hostname string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ns, err := net.DefaultResolver.LookupNS(ctx, "trycloudflare.com")
	if err != nil || len(ns) == 0 {
		return false
	}
	authoritative := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ns[0].Host, "53"))
	}}
	addrs, err := authoritative.LookupHost(ctx, hostname+".")
	return err == nil && len(addrs) > 0
}

// connect holds one connection to the edge until it drops or ctx ends, and
// says why it ended.
func (c *Client) connect(ctx context.Context, creds Credentials, edge string, attempts uint8, connected func(location string)) error {
	tunnelID, err := uuid.Parse(creds.ID)
	if err != nil {
		return fmt.Errorf("%w: bad tunnel id: %v", errRejected, err)
	}
	d := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second},
		Config:    &tls.Config{ServerName: edgeServerName, RootCAs: rootCAs(), MinVersion: tls.VersionTLS12},
	}
	conn, err := d.DialContext(ctx, "tcp", edge)
	if err != nil {
		return fmt.Errorf("reach the Cloudflare edge: %w", err)
	}

	// connCtx ends the socket. It outlives ctx by as long as unregistering
	// takes, so the edge is told before the socket goes.
	connCtx, closeConn := context.WithCancel(context.Background())
	defer closeConn()
	var registered atomic.Bool
	unregistered := make(chan struct{})
	go func() {
		select {
		case <-connCtx.Done():
		case <-ctx.Done():
			if registered.Load() {
				select {
				case <-unregistered:
				case <-time.After(5 * time.Second):
				}
			}
		}
		conn.Close()
	}()

	var mu sync.Mutex
	ended := errors.New("lost the connection to the Cloudflare edge")
	fail := func(err error) {
		mu.Lock()
		ended = err
		mu.Unlock()
		closeConn()
	}

	control := func(w http.ResponseWriter, r *http.Request) {
		reg, location, err := register(connCtx, &stream{r: r, w: w}, registerParams{
			creds: creds, tunnelID: tunnelID, clientID: c.clientID, version: c.version, attempts: attempts,
		})
		if err != nil {
			fail(err)
			return
		}
		defer reg.close()
		registered.Store(true)
		connected(location)
		select {
		case <-ctx.Done():
			reg.unregister(connCtx)
			close(unregistered)
		case <-reg.conn.Done():
			fail(errors.New("the Cloudflare edge closed the control stream"))
		case <-connCtx.Done():
		}
	}

	srv := &http2.Server{MaxConcurrentStreams: math.MaxUint32, ReadIdleTimeout: 30 * time.Second, PingTimeout: 15 * time.Second}
	srv.ServeConn(conn, &http2.ServeConnOpts{
		Context: connCtx,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(upgradeHeader) == "control-stream" {
				control(w, r)
				return
			}
			c.serve(w, r)
		}),
	})
	mu.Lock()
	defer mu.Unlock()
	return ended
}

// stream is the control stream as the byte pipe the RPC runs over: the edge
// writes into the request body, and we answer in the response body, flushed
// at once so a call is not left sitting in a buffer.
type stream struct {
	r *http.Request
	w http.ResponseWriter
}

func (s *stream) Read(p []byte) (int, error) { return s.r.Body.Read(p) }

func (s *stream) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if err == nil {
		s.w.(http.Flusher).Flush()
	}
	return n, err
}

func (s *stream) Close() error { return nil }
