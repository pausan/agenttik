package screen

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A pixel drawn over black and over white gives back its color and alpha.
func TestAlphaFromBackgrounds(t *testing.T) {
	// BGRX: opaque red, clear, half-transparent white.
	black := []byte{0, 0, 255, 0, 0, 0, 0, 0, 128, 128, 128, 0}
	white := []byte{0, 0, 255, 0, 255, 255, 255, 0, 255, 255, 255, 0}
	got := alphaFromBackgrounds(black, white)
	if want := []byte{255, 0, 0, 255, 0, 0, 0, 0, 255, 255, 255, 128}; !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// SendInput's 0–65535 coordinates land on the pixel they were made from.
func TestNormalizeLandsOnThePixel(t *testing.T) {
	for _, size := range []int{1366, 1920, 2560, 3840, 5120} {
		for p := 0; p < size; p++ {
			n := int(normalize(p, size))
			if n > 65535 || n*size/65536 != p {
				t.Fatalf("pixel %d of %d: %d lands on %d", p, size, n, n*size/65536)
			}
		}
	}
}

// Every key a browser names, other than characters, has a Windows key.
func TestWindowsKeysCoverBrowserKeys(t *testing.T) {
	for name, ks := range browserKeys {
		if _, ok := RuneForKeysym(ks); ok && ks != 0x20 {
			continue
		}
		if _, ok := winKeys[ks]; !ok {
			t.Errorf("%s (%#x) has no Windows key", name, ks)
		}
	}
}

// The live test uses this machine's own screen, pointer and clipboard, so
// it only runs when asked: AGENTTIK_SCREEN_LIVE=1.
func TestGDILive(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_LIVE") == "" {
		t.Skip("set AGENTTIK_SCREEN_LIVE=1 to use this machine's screen")
	}
	d, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

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
	t.Logf("screen %dx%d, %v a grab", w, h, time.Since(start)/10)

	_, _, serial, err := d.Cursor()
	shape, _ := d.CursorShape()
	if err != nil || shape.W == 0 || shape.Serial != serial || shape.HotX >= shape.W || shape.HotY >= shape.H {
		t.Fatalf("cursor: %v, %dx%d hot %d,%d", err, shape.W, shape.H, shape.HotX, shape.HotY)
	}

	t.Run("clipboard", func(t *testing.T) {
		saved, _ := d.Clipboard()
		defer d.SetClipboard(saved) //nolint:errcheck
		if err := d.SetClipboard("agenttik é 😀"); err != nil {
			t.Fatal(err)
		}
		if got, _ := d.Clipboard(); got != "agenttik é 😀" {
			t.Fatalf("read back %q", got)
		}
		changes, cancel := d.ClipboardChanges()
		defer cancel()
		if err := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard -Value 'from powershell'").Run(); err != nil {
			t.Fatal(err)
		}
		select {
		case text := <-changes:
			if strings.TrimSpace(text) != "from powershell" {
				t.Fatalf("announced %q", text)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("PowerShell's text was not announced")
		}
	})

	t.Run("pointer", func(t *testing.T) {
		x0, y0, _, _ := d.Cursor()
		defer d.Move(x0, y0) //nolint:errcheck
		if err := d.Move(200, 100); err != nil {
			t.Fatal(err)
		}
		x, y := 0, 0
		for end := time.Now().Add(time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			if x, y, _, _ = d.Cursor(); x == 200 && y == 100 {
				break
			}
		}
		if x != 200 || y != 100 {
			t.Fatalf("pointer at %d,%d after moving to 200,100", x, y)
		}
	})
}
