package remote

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryNetworks(t *testing.T) {
	var addresses []netip.Addr
	for _, raw := range []string{"10.2.3.4", "10.9.8.7", "172.20.1.2", "192.168.5.2", "127.0.0.1", "169.254.1.2", "8.8.8.8", "fd00::1", "::ffff:10.1.1.1"} {
		addresses = append(addresses, netip.MustParseAddr(raw))
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16")}
	if got := DiscoveryNetworks(addresses); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestHosts(t *testing.T) {
	if got := Hosts([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.168.0.0/16")}); got != 1<<24+1<<16 {
		t.Fatalf("hosts: %d", got)
	}
}

func TestScannerResumesUnfinishedAddresses(t *testing.T) {
	for _, beforeStart := range []bool{false, true} {
		t.Run(fmt.Sprint("cancelBeforeStart=", beforeStart), func(t *testing.T) {
			var resume atomic.Bool
			var answered atomic.Bool
			listener, err := net.Listen("tcp4", "0.0.0.0:0")
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !resume.Load() && !answered.CompareAndSwap(false, true) {
					<-r.Context().Done()
					return
				}
				fmt.Fprint(w, `{"application":"agenttik","version":"1","id":"office"}`)
			}))
			srv.Listener = listener
			srv.Start()
			defer srv.Close()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			scanner := NewScanner([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/24")})
			var probed atomic.Uint64
			matches := make(map[string]bool)
			ctx, cancel := context.WithCancel(context.Background())
			if beforeStart {
				cancel()
			}
			err = scanner.Run(ctx, port, &probed, func(target *url.URL, info Info) error {
				matches[target.Hostname()] = true
				cancel()
				return nil
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("pause: %v", err)
			}
			paused := probed.Load()
			if paused >= 256 || (beforeStart && paused != 0) {
				t.Fatalf("paused progress: %d", paused)
			}
			resume.Store(true)
			ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			err = scanner.Run(ctx, port, &probed, func(target *url.URL, info Info) error {
				if matches[target.Hostname()] {
					t.Errorf("repeated completed address: %s", target)
				}
				matches[target.Hostname()] = true
				return nil
			})
			if err != nil || probed.Load() != 256 || len(matches) != 256 {
				t.Fatalf("resume: err=%v probed=%d matches=%d", err, probed.Load(), len(matches))
			}
		})
	}
}
