package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

type discoveryResponse struct {
	Running  bool                `json:"running"`
	Complete bool                `json:"complete"`
	Probed   uint64              `json:"probed"`
	Total    uint64              `json:"total"`
	Port     int                 `json:"port"`
	Machines []discoveredMachine `json:"machines"`
	Error    string              `json:"error"`
}

func discovery(t *testing.T, s *Server, method string, body any) discoveryResponse {
	t.Helper()
	return decode[discoveryResponse](t, do(t, s, method, "/api/remotes/discovery", body))
}

func waitDiscovery(t *testing.T, s *Server, ready func(discoveryResponse) bool) discoveryResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := discovery(t, s, "GET", nil)
		if ready(state) {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("discovery did not reach expected state")
	return discoveryResponse{}
}

func TestMachineDiscoveryPauseConnectResumeAndRestart(t *testing.T) {
	local, st := newTestServer(t)
	self := machineID(t, st)
	local.remotes.networks = func() ([]netip.Prefix, error) {
		return []netip.Prefix{netip.MustParsePrefix("127.0.0.0/24")}, nil
	}
	var resume atomic.Bool
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if !resume.Load() && host != "127.0.0.0" && host != "127.0.0.1" {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/profiles" {
			fmt.Fprint(w, `{"profiles":[{"id":"default","name":"Default"}]}`)
			return
		}
		switch host {
		case "127.0.0.0":
			fmt.Fprintf(w, `{"application":"agenttik","version":"1","id":%q}`, self)
		case "127.0.0.1", "127.0.0.2":
			fmt.Fprint(w, `{"application":"agenttik","version":"1","id":"office","name":"Office"}`)
		case "127.0.0.3":
			fmt.Fprint(w, `{"application":"agenttik","version":"1"}`)
		case "127.0.0.5":
			fmt.Fprint(w, `{"application":"agenttik","version":"2","id":"lab","name":"Lab"}`)
		default:
			fmt.Fprint(w, `{"application":"other","version":"1"}`)
		}
	}))
	srv.Listener = listener
	srv.Start()
	defer srv.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	var portNumber int
	fmt.Sscan(port, &portNumber)
	state := discovery(t, local, "POST", map[string]any{"port": portNumber})
	if !state.Running || state.Total != 256 {
		t.Fatalf("start: %+v", state)
	}
	state = waitDiscovery(t, local, func(s discoveryResponse) bool { return len(s.Machines) == 1 })
	if state.Machines[0].ID != "office" {
		t.Fatalf("found: %+v", state)
	}

	// Adding a discovered machine pauses the scan before attempting sign-in.
	connection := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": "http://127.0.0.1:" + port}))
	if connection.Status != "ok" {
		t.Fatalf("connect: %+v", connection)
	}
	state = discovery(t, local, "GET", nil)
	if state.Running || state.Complete || len(state.Machines) != 1 || state.Probed >= 256 {
		t.Fatalf("pause on connect: %+v", state)
	}
	paused := state.Probed
	time.Sleep(20 * time.Millisecond)
	if got := discovery(t, local, "GET", nil); got.Probed != paused {
		t.Fatalf("paused scan still probed: %+v", got)
	}

	// Refreshing the saved machine also pauses a resumed search.
	discovery(t, local, "POST", nil)
	connection = decode[remoteState](t, do(t, local, "POST", "/api/remotes/office/connect", nil))
	if connection.Status != "ok" || discovery(t, local, "GET", nil).Running {
		t.Fatal("saved connection did not pause")
	}
	state = discovery(t, local, "DELETE", nil)
	if state.Running || state.Probed < paused {
		t.Fatalf("manual pause: %+v", state)
	}
	resume.Store(true)
	discovery(t, local, "POST", nil)
	state = waitDiscovery(t, local, func(s discoveryResponse) bool { return !s.Running })
	if !state.Complete || state.Probed != 256 || len(state.Machines) != 2 || state.Error != "" {
		t.Fatalf("resume: %+v", state)
	}
	// Discovery lists results, but only a chosen machine is saved.
	saved := decode[struct{ Remotes []remoteView }](t, do(t, local, "GET", "/api/remotes", nil))
	if len(saved.Remotes) != 1 {
		t.Fatalf("saved unchosen machines: %+v", saved)
	}

	resume.Store(false)
	state = discovery(t, local, "POST", map[string]any{"restart": true, "port": portNumber})
	if !state.Running || state.Probed >= 256 || len(state.Machines) >= 2 {
		t.Fatalf("restart: %+v", state)
	}
	state = discovery(t, local, "DELETE", nil)
	if state.Running || state.Complete {
		t.Fatalf("stop restarted scan: %+v", state)
	}
	discovery(t, local, "POST", nil)
	local.remotes.stop()
	if discovery(t, local, "GET", nil).Running {
		t.Fatal("shutdown left discovery running")
	}
}

func TestMachineDiscoveryRejectsInvalidSearches(t *testing.T) {
	local, _ := newTestServer(t)
	for _, port := range []int{-1, 65536} {
		res := do(t, local, "POST", "/api/remotes/discovery", map[string]any{"port": port})
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatalf("invalid port %d: %d", port, res.StatusCode)
		}
	}
	local.remotes.networks = func() ([]netip.Prefix, error) { return nil, nil }
	res := do(t, local, "POST", "/api/remotes/discovery", nil)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("no networks: %d", res.StatusCode)
	}
	if state := discovery(t, local, "GET", nil); state.Running || len(state.Machines) != 0 {
		t.Fatalf("initial state: %+v", state)
	}
}
