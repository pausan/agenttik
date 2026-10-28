package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"
	"time"
)

// addressRemote is a fake machine: it identifies as id after delay, lists
// its profiles and reports addresses as its own.
func addressRemote(t *testing.T, id string, delay time.Duration, addresses ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"id": id, "name": id, "version": "test", "application": "agenttik"})
		case "/api/profiles":
			json.NewEncoder(w).Encode(map[string]any{"profiles": []profileStatus{{Profile: Profile{ID: "default", Name: "Default"}}}})
		case "/api/server/addresses":
			json.NewEncoder(w).Encode(map[string]any{"addresses": addresses})
		case "/api/projects":
			w.Write([]byte(`{"from":"` + id + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// closedAddress is an address nothing listens on.
func closedAddress(t *testing.T) string {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func TestListenerAddresses(t *testing.T) {
	interfaces := []netip.Addr{
		netip.MustParseAddr("192.168.1.5"),
		netip.MustParseAddr("203.0.113.9"),
		netip.MustParseAddr("169.254.3.3"),
		netip.MustParseAddr("fd00::1"),
	}
	got := listenerAddresses([]string{"0.0.0.0:7717", "127.0.0.1:41000", "10.0.0.2:8080", "[::]:7717", "bad"}, interfaces)
	want := []string{"http://192.168.1.5:7717", "http://203.0.113.9:7717", "http://10.0.0.2:8080"}
	if !slices.Equal(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

func TestRemoteCandidatesPreferLocalThenRecent(t *testing.T) {
	r := savedRemote{
		Address:   "https://far.trycloudflare.com",
		Addresses: []string{"https://far.trycloudflare.com", "http://203.0.113.9:7717", "http://10.0.0.2:7717", "http://192.168.1.5:7717", "http://studio.local:7717"},
	}
	want := []string{"http://10.0.0.2:7717", "http://192.168.1.5:7717", "http://studio.local:7717", "https://far.trycloudflare.com", "http://203.0.113.9:7717"}
	if got := r.candidates(); !slices.Equal(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}

	r.use("http://192.168.1.5:7717")
	if r.Address != "http://192.168.1.5:7717" || r.Addresses[0] != r.Address || len(r.Addresses) != 5 {
		t.Fatalf("%+v", r)
	}
	for i := range 10 {
		r.learn([]string{"http://192.168.9." + string(rune('0'+i)) + ":7717", "not a url/path"})
	}
	if len(r.Addresses) != maxRemoteAddresses || r.Addresses[0] != "http://192.168.1.5:7717" {
		t.Fatalf("%v", r.Addresses)
	}
}

func TestLocateQuickCheck(t *testing.T) {
	fast := addressRemote(t, "lab", 0)
	slow := addressRemote(t, "lab", 600*time.Millisecond)
	silent := addressRemote(t, "lab", time.Hour)
	other := addressRemote(t, "elsewhere", 0)
	dead := closedAddress(t)
	locateIn := func(addresses ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		address, _, err := locate(ctx, savedRemote{ID: "lab", Address: addresses[0], Addresses: addresses})
		return address, err
	}

	if got, err := locateIn(dead, fast.URL); err != nil || got != fast.URL {
		t.Fatalf("dead first: %s %v", got, err)
	}
	// The better address answers after a worse one, within the grace.
	if got, err := locateIn(slow.URL, fast.URL); err != nil || got != slow.URL {
		t.Fatalf("slow better: %s %v", got, err)
	}
	// A better address that never answers costs the grace, no more.
	start := time.Now()
	if got, err := locateIn(silent.URL, fast.URL); err != nil || got != fast.URL {
		t.Fatalf("silent better: %s %v", got, err)
	}
	if took := time.Since(start); took > remoteStagger+remoteGrace+time.Second {
		t.Fatalf("took %v", took)
	}
	// The best address answering first does not wait for the others.
	start = time.Now()
	if got, err := locateIn(fast.URL, silent.URL); err != nil || got != fast.URL || time.Since(start) > remoteStagger {
		t.Fatalf("fast best: %s %v after %v", got, err, time.Since(start))
	}
	if _, err := locateIn(other.URL, dead); !errors.Is(err, errOtherMachine) {
		t.Fatalf("other machine: %v", err)
	}
	if _, err := locateIn(dead); err == nil || errors.Is(err, errOtherMachine) {
		t.Fatalf("dead: %v", err)
	}
}

func TestRemoteLearnsAddressesAndMovesBetweenThem(t *testing.T) {
	away := addressRemote(t, "lab", 0)
	home := addressRemote(t, "lab", 0, away.URL, "http://192.0.2.7:7717")
	s, _ := newTestServer(t)
	state := decode[remoteState](t, do(t, s, "POST", "/api/remotes", map[string]string{"address": home.URL}))
	if state.Status != "ok" || !slices.Equal(state.Remote.Addresses, []string{home.URL, away.URL, "http://192.0.2.7:7717"}) {
		t.Fatalf("%+v", state.Remote)
	}

	// Leaving home: the next request moves to the address that answers.
	home.Close()
	resp := do(t, s, "GET", "/api/projects?remote=lab", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != `{"from":"lab"}` {
		t.Fatalf("request after moving: %d %s", resp.StatusCode, body)
	}
	r, _ := s.remotes.get("lab")
	if r.Address != away.URL || r.Addresses[0] != away.URL || len(r.Addresses) != 3 {
		t.Fatalf("%+v", r)
	}

	// Adding an address the machine already has keeps the others.
	again := decode[remoteState](t, do(t, s, "POST", "/api/remotes", map[string]string{"address": away.URL}))
	if again.Status != "ok" || len(again.Remote.Addresses) != 3 {
		t.Fatalf("%+v", again.Remote)
	}
}
