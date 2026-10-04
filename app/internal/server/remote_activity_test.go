package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func activityRemote(t *testing.T, id string) (*httptest.Server, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	busy, status := &atomic.Bool{}, &atomic.Int32{}
	status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			json.NewEncoder(w).Encode(map[string]string{"id": id, "name": id, "version": "test", "application": "agenttik"})
			return
		}
		if r.URL.Path != "/api/profiles" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(int(status.Load()))
		json.NewEncoder(w).Encode(map[string]any{"profiles": []profileStatus{
			{Profile: Profile{ID: "default", Name: "Default"}},
			{Profile: Profile{ID: "work", Name: "Work"}, Busy: busy.Load()},
		}})
	}))
	t.Cleanup(srv.Close)
	return srv, busy, status
}

func refreshRemoteActivity(t *testing.T, m *remoteManager, id string) {
	t.Helper()
	r, err := m.get(id)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	a := m.activity[id]
	m.mu.Unlock()
	if a == nil {
		t.Fatal("connected remote has no activity worker")
	}
	m.refreshActivity(context.Background(), r, a)
}

func TestRemoteActivityTracksConnectedMachines(t *testing.T) {
	first, firstBusy, _ := activityRemote(t, "office")
	second, secondBusy, secondStatus := activityRemote(t, "lab")
	firstBusy.Store(true)
	secondBusy.Store(true)
	s, _ := newTestServer(t)
	state := decode[remoteState](t, do(t, s, "POST", "/api/remotes", map[string]string{"address": first.URL}))
	if state.Status != "ok" || !state.Remote.Profiles[1].Busy {
		t.Fatalf("connection lost activity: %+v", state)
	}
	edges := make(chan bool, 8)
	s.OnRemoteBusy(func(busy bool) { edges <- busy })
	wantEdge := func(want bool) {
		t.Helper()
		select {
		case got := <-edges:
			if got != want {
				t.Fatalf("activity edge = %v, want %v", got, want)
			}
		default:
			t.Fatalf("missing activity edge %v", want)
		}
	}
	wantEdge(true)
	decode[remoteState](t, do(t, s, "POST", "/api/remotes", map[string]string{"address": second.URL}))
	firstBusy.Store(false)
	refreshRemoteActivity(t, s.remotes, "office")
	select {
	case edge := <-edges:
		t.Fatalf("work still running on lab, got edge %v", edge)
	default:
	}
	// An expired sign-in clears activity even if the last known turn ran.
	secondStatus.Store(http.StatusUnauthorized)
	refreshRemoteActivity(t, s.remotes, "lab")
	wantEdge(false)
	secondStatus.Store(http.StatusOK)
	refreshRemoteActivity(t, s.remotes, "lab")
	wantEdge(true)
	list := decode[struct{ Remotes []remoteView }](t, do(t, s, "GET", "/api/remotes", nil))
	if list.Remotes[0].Profiles[1].Busy || !list.Remotes[1].Profiles[1].Busy {
		t.Fatalf("picker activity: %+v", list)
	}
	// Busy state is transient; restarting must not restore working dots.
	data, err := os.ReadFile(s.remotes.path)
	if err != nil || strings.Contains(string(data), `"busy"`) {
		t.Fatalf("persisted activity: %s, %v", data, err)
	}
	do(t, s, "DELETE", "/api/remotes/lab", nil).Body.Close()
	wantEdge(false)
}

func TestRemoteActivityIgnoresOldWorkersAndStops(t *testing.T) {
	remote, busy, status := activityRemote(t, "office")
	s, _ := newTestServer(t)
	state := decode[remoteState](t, do(t, s, "POST", "/api/remotes", map[string]string{"address": remote.URL}))
	if state.Status != "ok" {
		t.Fatalf("connection failed: %+v", state)
	}
	r, err := s.remotes.get("office")
	if err != nil {
		t.Fatal(err)
	}
	s.remotes.mu.Lock()
	old := s.remotes.activity[r.ID]
	s.remotes.mu.Unlock()
	busy.Store(true)
	decode[remoteState](t, do(t, s, "POST", "/api/remotes/office/connect", nil))
	status.Store(http.StatusServiceUnavailable)
	// An old request completing after reconnect cannot overwrite its state.
	s.remotes.refreshActivity(context.Background(), r, old)
	list := decode[struct{ Remotes []remoteView }](t, do(t, s, "GET", "/api/remotes", nil))
	if !list.Remotes[0].Profiles[1].Busy {
		t.Fatal("old worker overwrote reconnected activity")
	}
	s.remotes.stop()
	s.remotes.watchActivity(r, []profileStatus{{Profile: Profile{ID: "default"}, Busy: true}})
	s.remotes.mu.Lock()
	defer s.remotes.mu.Unlock()
	if len(s.remotes.activity) != 0 || s.remotes.activityBusy {
		t.Fatal("shutdown left activity running or allowed another worker")
	}
}
