package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestScanRemotes(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status, count int
	}{
		{"instance", `{"application":"agenttik","version":"1.2.3","name":"Office"}`, 200, 1},
		{"unrelated", `{"application":"other","version":"1"}`, 200, 0},
		{"missing version", `{"application":"agenttik"}`, 200, 0},
		{"redirect", `{"application":"agenttik","version":"1"}`, 302, 0},
		{"oversize", strings.Repeat("x", 4097), 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/version" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
			var out bytes.Buffer
			count, err := scanRemotes(context.Background(), []netip.Prefix{netip.PrefixFrom(netip.MustParseAddr(host), 32)}, port, &out)
			if err != nil || count != tc.count {
				t.Fatalf("count=%d err=%v", count, err)
			}
			if tc.count == 1 && !strings.Contains(out.String(), srv.URL+"\t\"Office\"\t\"1.2.3\"") {
				t.Fatalf("unexpected output: %q", out.String())
			}
		})
	}
}

func TestScanRemotesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err := scanRemotes(ctx, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, "7717", &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("canceled scan did not stop promptly")
	}
}

func TestScanRemotesDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	started := time.Now()
	count, err := scanRemotes(context.Background(), []netip.Prefix{netip.PrefixFrom(netip.MustParseAddr(host), 32)}, port, &bytes.Buffer{})
	if count != 0 || err != nil {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("probe exceeded deadline")
	}
}
