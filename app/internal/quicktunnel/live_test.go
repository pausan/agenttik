package quicktunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestLive opens a real quick tunnel on trycloudflare.com and checks a plain
// request, an SSE stream carried as a websocket and reusing the saved tunnel all work
// through it. It needs the internet, so it only runs when asked:
//
//	QUICKTUNNEL_LIVE=1 go test ./app/internal/quicktunnel -run Live -v
func TestLive(t *testing.T) {
	if os.Getenv("QUICKTUNNEL_LIVE") == "" {
		t.Skip("set QUICKTUNNEL_LIVE=1 to open a real tunnel")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Origin", "yes")
		fmt.Fprint(w, "hello")
	})
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
	})
	go http.Serve(ln, mux) //nolint:errcheck

	var saved Credentials
	// The lock wraps the origin and counts failed logins by visitor address.
	var visitor atomic.Value
	wrap := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			visitor.Store(r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}
	c := New(ln.Addr().String(), wrap, "test")
	began := time.Now()
	c.Start(Credentials{}, func(cr Credentials) { saved = cr })
	defer c.Stop()
	url := waitConnected(t, c)
	t.Logf("tunnel up at %s in %s after %s", url, c.Status().Location, time.Since(began))

	body := getWithRetry(t, url+"/hello")
	if body != "hello" {
		t.Fatalf("hello: %q", body)
	}
	if addr, _ := visitor.Load().(string); addr == "" || strings.HasPrefix(addr, "127.") {
		t.Fatalf("visitor address %q, want the public one", addr)
	}

	// SSE is held back by the edge until the response ends; the same
	// stream asked for as a websocket arrives event by event.
	start := time.Now()
	events := websocketEvents(t, strings.TrimPrefix(url, "https://"), "/sse")
	var got []string
	for ev := range events {
		got = append(got, ev)
		if len(got) == 1 && time.Since(start) > 2500*time.Millisecond {
			t.Fatalf("first event took %s: the stream is buffered", time.Since(start))
		}
	}
	if strings.Join(got, ",") != "0,1,2" {
		t.Fatalf("events %q", got)
	}

	// Reconnect with the saved tunnel: same address.
	c.Stop()
	c.Start(saved, func(cr Credentials) { t.Errorf("asked for a new tunnel: %s", cr.Hostname) })
	if again := waitConnected(t, c); again != url {
		t.Fatalf("reconnected at %s, want %s", again, url)
	}
	getWithRetry(t, url+"/hello")
}

// TestLiveStaleTunnel starts on a tunnel Cloudflare does not know, which is
// what a saved one looks like after a long time offline, and expects a new
// one to replace it within seconds rather than after the edge's own
// half-minute of retryable refusals.
func TestLiveStaleTunnel(t *testing.T) {
	if os.Getenv("QUICKTUNNEL_LIVE") == "" {
		t.Skip("set QUICKTUNNEL_LIVE=1 to open a real tunnel")
	}
	c := New("127.0.0.1:9", nil, "test")
	stale := Credentials{ID: "2d8f2f26-6f2b-4c39-9c0b-9b7a1f0e3c11", AccountTag: "x", Secret: []byte("0123456789abcdef0123456789abcdef"), Hostname: "gone.trycloudflare.com"}
	renewed := make(chan Credentials, 1)
	c.Start(stale, func(cr Credentials) { renewed <- cr })
	defer c.Stop()
	select {
	case cr := <-renewed:
		if cr.Hostname == stale.Hostname {
			t.Fatalf("kept %s", cr.Hostname)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("not replaced: %+v", c.Status())
	}
}

// websocketEvents opens path on host as a websocket and yields each text
// message until the server closes it.
func websocketEvents(t *testing.T, host, path string) <-chan string {
	t.Helper()
	ips, err := liveResolver.LookupHost(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.Dial("tcp", net.JoinHostPort(ips[0], "443"), &tls.Config{ServerName: host, NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", path, host)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols || resp.Header.Get("Sec-Websocket-Accept") != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("upgrade: %s %v", resp.Status, resp.Header)
	}
	out := make(chan string)
	go func() {
		defer close(out)
		defer conn.Close()
		for {
			var hdr [2]byte
			if _, err := io.ReadFull(br, hdr[:]); err != nil {
				return
			}
			n := int(hdr[1] & 0x7f) // the test's messages are all short
			payload := make([]byte, n)
			if _, err := io.ReadFull(br, payload); err != nil {
				return
			}
			switch hdr[0] & 0x0f {
			case opText:
				out <- string(payload)
			case opClose:
				return
			}
		}
	}()
	return out
}

// liveClient resolves through 1.1.1.1: a hostname minted a moment ago is
// often cached as missing by a local resolver that was asked too early.
var liveResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, "1.1.1.1:53")
}}

var liveClient = &http.Client{Transport: &http.Transport{DialContext: (&net.Dialer{Resolver: liveResolver}).DialContext}}

func waitConnected(t *testing.T, c *Client) string {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if st := c.Status(); st.Ready {
			return st.URL
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("not connected: %+v", c.Status())
	return ""
}

// getWithRetry waits out DNS for a hostname that was minted a moment ago.
func getWithRetry(t *testing.T, url string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := liveClient.Do(req)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if resp.Header.Get("X-Origin") != "yes" {
					t.Fatalf("origin header lost: %v", resp.Header)
				}
				return string(b)
			}
			err = fmt.Errorf("%s: %s", resp.Status, b)
		}
		if ctx.Err() != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		t.Logf("GET %s: %v (retrying)", url, err)
		time.Sleep(2 * time.Second)
	}
}
