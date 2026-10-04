package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pausan/agenttik/app/internal/remote"
)

const remoteActivityInterval = 3 * time.Second

// Activity is kept in memory and watched only after a successful connection.
// Each connected machine has one worker, independent of the visible window.
type remoteActivity struct {
	cancel   context.CancelFunc
	profiles []profileStatus
}

func (m *remoteManager) watchActivity(r savedRemote, profiles []profileStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	// A removed machine must not be revived by an in-flight connection.
	found := false
	for _, saved := range m.remotes {
		if saved.ID == r.ID && saved.Address == r.Address {
			found = true
			break
		}
	}
	if !found {
		return
	}
	if m.activity == nil {
		m.activity = make(map[string]*remoteActivity)
	}
	if old := m.activity[r.ID]; old != nil {
		old.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &remoteActivity{cancel: cancel, profiles: profiles}
	m.activity[r.ID] = a
	m.activityChangedLocked()
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		ticker := time.NewTicker(remoteActivityInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.refreshActivity(ctx, r, a)
			}
		}
	}()
}

func (m *remoteManager) refreshActivity(parent context.Context, initial savedRemote, a *remoteActivity) {
	ctx, cancel := context.WithTimeout(parent, remoteActivityInterval)
	defer cancel()
	var profiles []profileStatus
	r, err := m.get(initial.ID)
	if err == nil && r.Address == initial.Address {
		// Do not attribute work to a different machine at the saved address.
		_, info, err := remote.Check(ctx, r.Address)
		if err == nil && info.ID == r.ID {
			res, err := m.request(ctx, r, "GET", "/api/profiles", nil, http.Header{"Accept": {"application/json"}})
			if err == nil {
				var list struct {
					Profiles []profileStatus `json:"profiles"`
				}
				if res.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&list) == nil {
					profiles = list.Profiles
				}
				res.Body.Close()
			}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activity[initial.ID] != a {
		return
	}
	if profiles == nil {
		// Keep names through disconnects, but never leave a stale working dot.
		profiles = append([]profileStatus{}, a.profiles...)
		for i := range profiles {
			profiles[i].Busy = false
		}
	}
	a.profiles = profiles
	m.activityChangedLocked()
}

func (m *remoteManager) activityChangedLocked() {
	busy := false
	for _, a := range m.activity {
		for _, p := range a.profiles {
			busy = busy || p.Busy
		}
	}
	if busy != m.activityBusy {
		m.activityBusy = busy
		if m.onBusy != nil {
			m.onBusy(busy)
		}
	}
}

// OnRemoteBusy reports the first busy and last idle edge across connected
// machines. Like OnBusy, the listener must not block and is registered once.
func (s *Server) OnRemoteBusy(fn func(bool)) {
	if s.remotes == nil {
		return
	}
	m := s.remotes
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onBusy = fn
	if m.activityBusy {
		fn(true)
	}
}

func (m *remoteManager) stopActivity() {
	m.mu.Lock()
	m.stopped = true
	for _, a := range m.activity {
		a.cancel()
	}
	m.activity = nil
	m.activityChangedLocked()
	m.mu.Unlock()
	m.workers.Wait()
}
