//go:build desktop

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
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
