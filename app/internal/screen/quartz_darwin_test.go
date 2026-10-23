package screen

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
)

func TestUnpremultiply(t *testing.T) {
	px := []byte{64, 32, 0, 128, 10, 20, 30, 255, 0, 0, 0, 0}
	unpremultiply(px)
	if want := []byte{127, 63, 0, 128, 10, 20, 30, 255, 0, 0, 0, 0}; !bytes.Equal(px, want) {
		t.Fatalf("got %v, want %v", px, want)
	}
}

func TestUTF16Of(t *testing.T) {
	if got := utf16Of('é'); len(got) != 1 || got[0] != 0xe9 {
		t.Fatalf("é: %x", got)
	}
	if got := utf16Of('😀'); len(got) != 2 || got[0] != 0xd83d || got[1] != 0xde00 {
		t.Fatalf("😀: %x", got)
	}
}

// Every key a browser names, other than characters, has a Mac key, except
// the few a Mac keyboard lacks.
func TestMacKeysCoverBrowserKeys(t *testing.T) {
	missing := map[uint32]bool{0xff13: true, 0xff14: true, 0xff61: true, 0xff7f: true} // Pause, ScrollLock, PrintScreen, NumLock
	for name, ks := range browserKeys {
		if _, ok := RuneForKeysym(ks); ok && ks != 0x20 {
			continue
		}
		_, special := macSpecialKeys[ks]
		_, mod := macModifiers[ks]
		if !special && !mod && !missing[ks] {
			t.Errorf("%s (%#x) has no Mac key", name, ks)
		}
	}
}

// The live test uses this Mac's own screen, pointer and clipboard, so it
// only runs when asked: AGENTTIK_SCREEN_LIVE=1. Input checks also need
// Accessibility for the process running the test.
func TestQuartzLive(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_LIVE") == "" {
		t.Skip("set AGENTTIK_SCREEN_LIVE=1 to use this Mac's screen")
	}
	d, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	q := d.(*quartz)

	w, h := d.Size()
	pix, gw, gh, err := d.Grab(nil)
	if err != nil || gw != w || gh != h || len(pix) != w*h*4 {
		t.Fatalf("grab: %v, %dx%d for a %dx%d screen, %d bytes", err, gw, gh, w, h, len(pix))
	}
	if bytes.Count(pix, []byte{0}) == len(pix) {
		t.Fatal("the screen grabbed black")
	}
	start := time.Now()
	for range 10 {
		d.Grab(pix)
	}
	t.Logf("screen %dx%d, %.1f pixels per point, %v a grab", w, h, q.scale, time.Since(start)/10)

	_, _, serial, err := d.Cursor()
	shape, _ := d.CursorShape()
	if err != nil || shape.W == 0 || shape.Serial != serial || shape.HotX >= shape.W || shape.HotY >= shape.H {
		t.Fatalf("cursor: %v, %dx%d hot %d,%d", err, shape.W, shape.H, shape.HotX, shape.HotY)
	}
	if !hasOpaque(shape.RGBA) {
		t.Fatal("the cursor is invisible")
	}

	keymap := readKeymap()
	for _, r := range "az09" {
		if _, ok := keymap[r]; !ok {
			t.Errorf("the layout has no key for %q", r)
		}
	}

	// A new resolution restarts the stream while frames arrive.
	t.Run("restart", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 5 {
				q.mu.Lock()
				err := q.startLocked()
				q.mu.Unlock()
				if err != nil {
					t.Error(err)
					return
				}
				if _, _, _, err := d.Grab(pix); err != nil {
					t.Error(err)
				}
			}
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("restarting the stream hung")
		}
	})

	t.Run("clipboard", func(t *testing.T) {
		saved, _ := d.Clipboard()
		defer d.SetClipboard(saved) //nolint:errcheck
		if err := d.SetClipboard("agenttik é 😀"); err != nil {
			t.Fatal(err)
		}
		if out, _ := exec.Command("pbpaste").Output(); string(out) != "agenttik é 😀" {
			t.Fatalf("pbpaste read %q", out)
		}
		changes, cancel := d.ClipboardChanges()
		defer cancel()
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader("from pbcopy")
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		select {
		case text := <-changes:
			if text != "from pbcopy" {
				t.Fatalf("announced %q", text)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("pbcopy's text was not announced")
		}
	})

	t.Run("input", func(t *testing.T) {
		if reason := q.ControlReason(); reason != "" {
			t.Skip(reason)
		}
		x0, y0, _, _ := d.Cursor()
		defer d.Move(x0, y0) //nolint:errcheck
		if err := d.Move(200, 100); err != nil {
			t.Fatal(err)
		}
		// Posted events reach the system a moment later.
		x, y := 0, 0
		for end := time.Now().Add(time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			if x, y, _, _ = d.Cursor(); x == 200 && y == 100 {
				break
			}
		}
		if x != 200 || y != 100 {
			t.Fatalf("pointer at %d,%d after moving to 200,100", x, y)
		}
		// Shift alone does nothing on screen; the system's own state says it
		// is held.
		if err := d.Key(0xffe1, true); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		held := flagsState()&flagShift != 0
		d.Key(0xffe1, false) //nolint:errcheck
		if !held {
			t.Fatal("Shift did not reach the system")
		}
		time.Sleep(50 * time.Millisecond)
		if flagsState()&flagShift != 0 {
			t.Fatal("Shift is still held")
		}
	})
}

func hasOpaque(rgba []byte) bool {
	for i := 3; i < len(rgba); i += 4 {
		if rgba[i] > 200 {
			return true
		}
	}
	return false
}

// flagsState is the modifiers the system believes held.
func flagsState() uint64 {
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL)
	if err != nil {
		return 0
	}
	var state func(int32) uint64
	purego.RegisterLibFunc(&state, lib, "CGEventSourceFlagsState")
	return state(1) // kCGEventSourceStateHIDSystemState
}
