//go:build desktop

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"testing"
)

func TestTrayImages(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayPNG))
	if err != nil {
		t.Fatalf("decode tray PNG: %v", err)
	}
	if got := img.Bounds().Size(); got.X != 64 || got.Y != 64 {
		t.Fatalf("tray PNG is %dx%d, want 64x64", got.X, got.Y)
	}

	// The logo's main mark is green. This catches rasterizers that preserve
	// the dark plate but silently drop the SVG gradients and filtered marks.
	colorPixels := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0x8000 && g > r+0x1000 && g > b+0x1000 {
				colorPixels++
			}
		}
	}
	if colorPixels < 100 {
		t.Fatalf("tray PNG lost its green mark: only %d green pixels", colorPixels)
	}

	if len(trayICO) < 6 || !bytes.Equal(trayICO[:4], []byte{0, 0, 1, 0}) {
		t.Fatal("tray ICO has an invalid header")
	}
	if count := binary.LittleEndian.Uint16(trayICO[4:6]); count < 4 {
		t.Fatalf("tray ICO has %d sizes, want at least 4", count)
	}
}

func TestMacOSDesktopShortcutUsesCommand(t *testing.T) {
	if got := desktopShortcutForOS("Ctrl+Shift+A", "darwin"); got != "Cmd+Shift+A" {
		t.Fatalf("macOS shortcut = %q", got)
	}
	if got := desktopShortcutForOS("Ctrl+Shift+A", "windows"); got != "Ctrl+Shift+A" {
		t.Fatalf("Windows shortcut = %q", got)
	}
}

func TestTrayShortcutFailure(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			w := &window{}
			stop := w.startTrayShortcut("Cmd+Shift+A", goos, func(string, func(bool)) (func(), error) {
				return nil, errors.New("permission denied")
			})
			if (stop != nil) != (goos == "darwin") {
				t.Fatalf("tray continues = %v on %s", stop != nil, goos)
			}
			if !strings.Contains(w.trayStatus(), "permission denied") {
				t.Fatalf("missing registration error: %q", w.trayStatus())
			}
			if stop != nil {
				if !strings.Contains(w.trayStatus(), "tray is active") {
					t.Fatalf("missing partial availability message: %q", w.trayStatus())
				}
				stop() // Missing permission must also be safe at shutdown.
			}
		})
	}
}

func TestTrayShortcutCleanup(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			w := &window{}
			stopped := false
			stop := w.startTrayShortcut("Ctrl+Shift+A", goos, func(chord string, toggle func(bool)) (func(), error) {
				if chord != "Ctrl+Shift+A" || toggle == nil {
					t.Fatal("missing shortcut registration arguments")
				}
				return func() { stopped = true }, nil
			})
			if stop == nil || w.trayStatus() != "" {
				t.Fatal("successful registration disabled tray or reported an error")
			}
			stop()
			if !stopped {
				t.Fatal("shortcut not unregistered")
			}
		})
	}
}

func TestQuitConfirmation(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		for _, busy := range []bool{false, true} {
			for _, accept := range []bool{false, true} {
				t.Run(fmt.Sprintf("explicit=%v/busy=%v/accept=%v", explicit, busy, accept), func(t *testing.T) {
					calls := 0
					w := &window{quitting: explicit, trayReady: explicit, busy: func() bool { return busy }}
					w.confirmQuit = func(ctx context.Context) bool {
						calls++
						if !w.beforeClose(ctx) {
							t.Fatal("overlapping close should be blocked")
						}
						return accept
					}
					blocked := w.beforeClose(context.Background())
					if blocked != (busy && !accept) || w.quitting == blocked {
						t.Fatalf("blocked=%v quitting=%v", blocked, w.quitting)
					}
					if (calls == 1) != busy {
						t.Fatalf("confirmation calls = %d", calls)
					}
					if blocked {
						w.busy = func() bool { return false }
						w.trayReady = false
						if w.beforeClose(context.Background()) {
							t.Fatal("cancel prevented a later idle close")
						}
					}
				})
			}
		}
	}
}
