package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/netauth"
)

// TestTunnelLive is the public address end to end, on the real
// trycloudflare.com: turned on through the API, locked, signed into from
// outside, and the UI's event stream read through it as a websocket. It needs
// the internet, so it only runs when asked:
//
//	QUICKTUNNEL_LIVE=1 go test ./app/internal/server -run TunnelLive -v
func TestTunnelLive(t *testing.T) {
	if os.Getenv("QUICKTUNNEL_LIVE") == "" {
		t.Skip("set QUICKTUNNEL_LIVE=1 to open a real tunnel")
	}
	s, _ := exposed(t)
	secret := lockTheServer(t, s)

	info := decode[serverConfigInfo](t, do(t, s, "PUT", "/api/server/tunnel", map[string]any{"enabled": true}))
	if info.Tunnel == nil || !info.Tunnel.Enabled {
		t.Fatalf("tunnel not on: %+v", info.Tunnel)
	}
	var public string
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(time.Second) {
		info = decode[serverConfigInfo](t, do(t, s, "GET", "/api/server", nil))
		if info.Tunnel.Ready {
			public = info.Tunnel.URL
			break
		}
	}
	if public == "" {
		t.Fatalf("never ready: %+v", info.Tunnel)
	}
	t.Logf("public at %s", public)
	if cfg, _ := s.store.GetServerConfig(); !strings.Contains(cfg.TunnelCredentials, strings.TrimPrefix(public, "https://")) {
		t.Fatalf("credentials not saved: %q", cfg.TunnelCredentials)
	}

	c := browser()
	c.Timeout = 30 * time.Second
	if resp := fetch(t, c, public+"/api/projects", "*/*"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("locked api status %d", resp.StatusCode)
	}
	code, _ := netauth.Code(secret, time.Now())
	login, err := c.PostForm(public+"/__auth/login", url.Values{"password": {exposedPassword}, "code": {code}, "next": {"/"}})
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status %d", login.StatusCode)
	}
	if resp := fetch(t, c, public+"/api/projects", "*/*"); resp.StatusCode != http.StatusOK {
		t.Fatalf("api status %d after login", resp.StatusCode)
	}
	u, _ := url.Parse(public)
	cookie := c.Jar.Cookies(u)

	// Without the cookie the stream is refused, with it events arrive.
	if status := websocketStatus(t, u.Host, "/api/stream", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous stream status %d", status)
	}
	msgs := websocketMessages(t, u.Host, "/api/stream", cookie)
	time.Sleep(time.Second) // let the subscription settle before publishing
	do(t, s, "POST", "/api/projects", map[string]string{"path": t.TempDir()})
	select {
	case m := <-msgs:
		if !strings.Contains(m, "projects") {
			t.Fatalf("event %q", m)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no event through the tunnel")
	}
	s.tunnel.Stop()
}

func websocketDial(t *testing.T, host, path string, cookies []*http.Cookie) (*http.Response, *bufio.Reader, net.Conn) {
	t.Helper()
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, "1.1.1.1:53")
	}}
	ips, err := resolver.LookupHost(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.Dial("tcp", net.JoinHostPort(ips[0], "443"), &tls.Config{ServerName: host, NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	var cookieLine string
	for _, ck := range cookies {
		cookieLine += "Cookie: " + ck.String() + "\r\n"
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n%s\r\n", path, host, cookieLine)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	return resp, br, conn
}

func websocketStatus(t *testing.T, host, path string, cookies []*http.Cookie) int {
	resp, _, conn := websocketDial(t, host, path, cookies)
	conn.Close()
	return resp.StatusCode
}

// websocketMessages yields each text message; pings are skipped.
func websocketMessages(t *testing.T, host, path string, cookies []*http.Cookie) <-chan string {
	resp, br, conn := websocketDial(t, host, path, cookies)
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	out := make(chan string, 16)
	t.Cleanup(func() { conn.Close() })
	go func() {
		for {
			var hdr [2]byte
			if _, err := io.ReadFull(br, hdr[:]); err != nil {
				return
			}
			n := int(hdr[1] & 0x7f)
			if n == 126 {
				var ext [2]byte
				io.ReadFull(br, ext[:])
				n = int(ext[0])<<8 | int(ext[1])
			}
			payload := make([]byte, n)
			if _, err := io.ReadFull(br, payload); err != nil {
				return
			}
			if hdr[0]&0x0f == 0x1 {
				out <- string(payload)
			}
		}
	}()
	return out
}
