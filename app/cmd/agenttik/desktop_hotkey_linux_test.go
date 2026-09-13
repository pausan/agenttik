//go:build desktop && linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestXGBLogWriter(t *testing.T) {
	var got bytes.Buffer
	w := xgbLogWriter{dst: &got}

	message := []byte("XGB: xgb.go:526: Invalid event/error type: <nil>\n")
	if n, err := w.Write(message); err != nil || n != len(message) {
		t.Fatalf("discarded write: n=%d err=%v", n, err)
	}
	if got.Len() != 0 {
		t.Fatalf("closed-event diagnostic was forwarded: %q", got.String())
	}

	other := []byte("XGB: a real error\n")
	if n, err := w.Write(other); err != nil || n != len(other) {
		t.Fatalf("forwarded write: n=%d err=%v", n, err)
	}
	if got.String() != string(other) {
		t.Fatalf("forwarded %q, want %q", got.String(), other)
	}
}

// Run on a private X server: xvfb-run go test -tags "desktop production webkit2_41" ./app/cmd/agenttik -run TestGlobalToggle
func TestGlobalToggle(t *testing.T) {
	if os.Getenv("AGENTTIK_TEST_HOTKEY") != "1" {
		t.Skip("needs a private X display and AGENTTIK_TEST_HOTKEY=1")
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		t.Skip("needs xdotool")
	}
	events := make(chan bool, 10)
	stop, err := registerToggle("Ctrl+Shift+A", func(foreground bool) { events <- foreground })
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()
	other, err := registerToggle("Ctrl+Shift+A", func(bool) {})
	if err == nil {
		other()
		t.Fatal("accepted a conflicting shortcut")
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("xdotool", args...).CombinedOutput(); err != nil {
			t.Fatalf("xdotool: %s: %v", out, err)
		}
	}
	run("keydown", "ctrl+shift+a")
	select {
	// The private display has no window of ours in front, and answering that
	// takes X round trips from the same goroutine that reads the key events.
	case foreground := <-events:
		if foreground {
			t.Fatal("reported this process as the foreground application")
		}
	case <-time.After(time.Second):
		t.Fatal("shortcut did not fire")
	}
	// Include real X11 auto-repeat while the chord is held.
	time.Sleep(1200 * time.Millisecond)
	run("keyup", "ctrl+shift+a")
	time.Sleep(100 * time.Millisecond)
	select {
	case <-events:
		t.Fatal("holding shortcut toggled again")
	default:
	}
	for i := 0; i < 3; i++ {
		run("key", "ctrl+shift+a")
		select {
		case <-events:
		case <-time.After(time.Second):
			t.Fatal("next press did not fire")
		}
	}
	stop()
	stopped = true
	stop, err = registerToggle("Ctrl+Shift+A", func(bool) {})
	if err != nil {
		t.Fatalf("shortcut was not released: %v", err)
	}
	stop()
}
