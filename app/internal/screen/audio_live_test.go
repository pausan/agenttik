//go:build windows || darwin

package screen

import (
	"os"
	"testing"
	"time"
)

// TestSoundLive records this machine's sound for a quarter of a second and
// checks that Close ends a waiting Read. On a Mac it needs Screen & System
// Audio Recording. Packets come whether or not anything plays: what the
// system leaves out is filled with silence.
func TestSoundLive(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_LIVE") == "" {
		t.Skip("AGENTTIK_SCREEN_LIVE=1 records this machine's sound")
	}
	a, err := openAudio()
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, 2*960)
	start := time.Now()
	loud := 0
	for range 12 {
		if err := a.Read(pcm); err != nil {
			t.Fatal(err)
		}
		if !silent(pcm) {
			loud++
		}
	}
	t.Logf("12 packets in %v, %d with sound", time.Since(start), loud)
	done := make(chan error)
	go func() {
		for {
			if err := a.Read(pcm); err != nil {
				done <- err
				return
			}
		}
	}()
	a.Close()
	select {
	case err := <-done:
		if err != errAudioClosed {
			t.Fatalf("read after close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not end Read")
	}
}
