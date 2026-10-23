package screen

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestPulseLive records a tone played into a sink of its own, which nobody
// hears, and checks that Close wakes a Read that waits. It needs a sound
// server and pactl and paplay.
func TestPulseLive(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_LIVE") == "" {
		t.Skip("AGENTTIK_SCREEN_LIVE=1 uses this machine's sound server")
	}
	out, err := exec.Command("pactl", "load-module", "module-null-sink", "sink_name=agenttik_test").Output()
	if err != nil {
		t.Skip("no null sink:", err)
	}
	t.Cleanup(func() { exec.Command("pactl", "unload-module", strings.TrimSpace(string(out))).Run() })

	a, err := openPulse("agenttik_test.monitor")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	play := exec.Command("paplay", "--device=agenttik_test", "--raw", "--format=s16le", "--rate=48000", "--channels=2")
	play.Stdin = strings.NewReader(string(tone(2 * time.Second)))
	if err := play.Start(); err != nil {
		t.Skip("no paplay:", err)
	}
	defer play.Process.Kill()

	pcm := make([]int16, 2*960)
	loud := 0
	for i := 0; i < 50 && loud < 10; i++ {
		if err := a.Read(pcm); err != nil {
			t.Fatal(err)
		}
		if !silent(pcm) {
			loud++
		}
	}
	if loud < 10 {
		t.Fatalf("heard %d frames of the tone in a second", loud)
	}

	done := make(chan error)
	go func() {
		for {
			if err := a.Read(pcm); err != nil {
				done <- err
				return
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	a.Close()
	select {
	case err := <-done:
		if err != errAudioClosed {
			t.Fatalf("read after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wake Read")
	}
}

// tone is d of a 440 Hz sine as raw 48 kHz 16-bit stereo.
func tone(d time.Duration) []byte {
	n := int(d.Seconds() * 48000)
	out := make([]byte, 4*n)
	for i := 0; i < n; i++ {
		v := uint16(int16(8000 * math.Sin(2*math.Pi*440*float64(i)/48000)))
		binary.LittleEndian.PutUint16(out[4*i:], v)
		binary.LittleEndian.PutUint16(out[4*i+2:], v)
	}
	return out
}
