package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRemoteDownIsTriedOnceEachPause(t *testing.T) {
	m := &remoteManager{}
	off := errors.New("off")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	m.noteReach(canceled, "a", off)
	if m.isDown("a") {
		t.Fatal("a request the caller gave up on marked the machine down")
	}
	late, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	m.noteReach(late, "a", off)
	if skip, try := m.skip("a"); !m.isDown("a") || !skip || try {
		t.Fatalf("a machine that took too long: down %v skip %v try %v", m.isDown("a"), skip, try)
	}

	m.down["a"] = time.Now().Add(-time.Second)
	if skip, try := m.skip("a"); !skip || !try {
		t.Fatalf("first after the pause: skip %v try %v", skip, try)
	}
	if skip, try := m.skip("a"); !skip || try {
		t.Fatalf("second after the pause: skip %v try %v", skip, try)
	}

	m.noteReach(context.Background(), "a", nil)
	if skip, _ := m.skip("a"); skip || m.isDown("a") {
		t.Fatal("a machine that answered is still down")
	}
}
