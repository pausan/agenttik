package server

import (
	"context"
	"errors"
	"time"
)

// A machine that stops answering is down until it answers again. Nothing
// waits on it meanwhile: it is asked again this often, in the background, or
// when the user opens one of its profiles.
const remoteRetry = 60 * time.Second

// noteReach records whether a remote answered. A request the caller gave up
// on says nothing about the machine; one that took too long does.
func (m *remoteManager) noteReach(ctx context.Context, id string, err error) {
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	m.downMu.Lock()
	defer m.downMu.Unlock()
	if err == nil {
		delete(m.down, id)
		return
	}
	if m.down == nil {
		m.down = map[string]time.Time{}
	}
	m.down[id] = time.Now().Add(remoteRetry)
}

func (m *remoteManager) isDown(id string) bool {
	m.downMu.Lock()
	defer m.downMu.Unlock()
	_, down := m.down[id]
	return down
}

// skip reports whether a background caller should leave a remote alone. Once
// its pause is over, the first to ask is told to try it; the rest keep
// skipping it.
func (m *remoteManager) skip(id string) (skip, try bool) {
	m.downMu.Lock()
	defer m.downMu.Unlock()
	retry, down := m.down[id]
	if !down {
		return false, false
	}
	if time.Now().Before(retry) {
		return true, false
	}
	m.down[id] = time.Now().Add(remoteRetry)
	return true, true
}
