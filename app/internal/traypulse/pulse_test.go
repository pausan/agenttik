package traypulse

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"testing"
)

// trayIcon is the real embedded tray export, so the thresholds here are
// checked against the icon that ships rather than a synthetic one.
func trayIcon(t *testing.T) []byte {
	t.Helper()
	icon, err := os.ReadFile("../../cmd/agenttik/tray.png")
	if err != nil {
		t.Fatalf("read tray icon: %v", err)
	}
	return icon
}

// markLight is the total light in the bright part of the icon: the sparkle and
// its halo, ignoring the plate. It is what the pulse moves.
func markLight(t *testing.T, frame []byte) float32 {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	rgba := toRGBA(img)
	var total float32
	for i := 0; i < rgba.Rect.Dx()*rgba.Rect.Dy(); i++ {
		p := rgba.Pix[i*4 : i*4+4 : i*4+4]
		if y := luma(p[0], p[1], p[2]); y > glowLo {
			total += y
		}
	}
	return total
}

func TestFramesRiseFromDarkToBright(t *testing.T) {
	icon := trayIcon(t)
	frames, err := Frames(icon, 7)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if len(frames) != 7 {
		t.Fatalf("got %d frames, want 7", len(frames))
	}

	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("decode icon: %v", err)
	}
	for i, frame := range frames {
		img, err := png.Decode(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("frame %d does not decode: %v", i, err)
		}
		if img.Bounds() != image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()) {
			t.Fatalf("frame %d is %v, want the icon's %v", i, img.Bounds(), src.Bounds())
		}
	}

	// Monotonic, so playing the frames in order is a rise with no wobble.
	for i := 1; i < len(frames); i++ {
		if prev, cur := markLight(t, frames[i-1]), markLight(t, frames[i]); cur <= prev {
			t.Fatalf("frame %d is not brighter than %d: %.0f then %.0f", i, i-1, prev, cur)
		}
	}

	// The cycle has to straddle the resting icon: caught at either end, a
	// working tray should not look like an idle one.
	resting := markLight(t, icon)
	if dimmest := markLight(t, frames[0]); dimmest >= resting {
		t.Fatalf("the dimmest frame (%.0f) is not below the resting icon (%.0f)", dimmest, resting)
	}
	if brightest := markLight(t, frames[len(frames)-1]); brightest <= resting*1.1 {
		t.Fatalf("the brightest frame (%.0f) barely clears the resting icon (%.0f)", brightest, resting)
	}
}

func TestFramesCycleThroughDarkBlueAndWhite(t *testing.T) {
	icon := trayIcon(t)
	frames, err := Frames(icon, 7)
	if err != nil {
		t.Fatal(err)
	}
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatal(err)
	}
	base := toRGBA(src)
	for _, stage := range []struct {
		index int
		name  string
		check func(r, g, b uint8) bool
	}{
		{0, "near-black", func(r, g, b uint8) bool { return r < 20 && g < 20 && b < 20 }},
		{3, "blue", func(r, g, b uint8) bool { return b > 240 && g > 100 && g < 150 && r < 50 }},
		{6, "white", func(r, g, b uint8) bool { return r > 250 && g > 250 && b > 250 }},
	} {
		img, err := png.Decode(bytes.NewReader(frames[stage.index]))
		if err != nil {
			t.Fatal(err)
		}
		frame := toRGBA(img)
		checked := 0
		for i := 0; i < len(base.Pix); i += 4 {
			p := base.Pix[i : i+4]
			// Check the solid green star in the original asset.
			if p[3] != 255 || p[1] < 200 || p[0] > 100 || p[2] > 180 {
				continue
			}
			q := frame.Pix[i : i+4]
			if !stage.check(q[0], q[1], q[2]) {
				t.Fatalf("%s star pixel = %v", stage.name, q)
			}
			checked++
		}
		if checked < 100 {
			t.Fatalf("only checked %d star pixels", checked)
		}
	}
}

// The plate is what makes the icon readable against a panel of any colour, so
// the glow must not creep across it.
func TestFramesLeaveThePlateAlone(t *testing.T) {
	frames, err := Frames(trayIcon(t), 7)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	peak, err := png.Decode(bytes.NewReader(frames[len(frames)-1]))
	if err != nil {
		t.Fatalf("decode peak: %v", err)
	}
	rgba := toRGBA(peak)
	// The corners are plate, far from the mark at the centre.
	for _, pt := range []image.Point{{2, 2}, {61, 2}, {2, 61}, {61, 61}} {
		p := rgba.PixOffset(pt.X, pt.Y)
		if y := luma(rgba.Pix[p], rgba.Pix[p+1], rgba.Pix[p+2]); y > markLo {
			t.Fatalf("the glow reached the plate at %v: luma %.2f", pt, y)
		}
	}
}

func TestFramesRejectsBadInput(t *testing.T) {
	if _, err := Frames(trayIcon(t), 1); err == nil {
		t.Fatal("a single frame is not a pulse, want an error")
	}
	if _, err := Frames([]byte("not a png"), 7); err == nil {
		t.Fatal("want an error for an undecodable icon")
	}
}

func TestICOHoldsEveryRequestedSize(t *testing.T) {
	frames, err := Frames(trayIcon(t), 2)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	sizes := []int{16, 32, 64}
	ico, err := ICO(frames[0], sizes...)
	if err != nil {
		t.Fatalf("ICO: %v", err)
	}

	if got := binary.LittleEndian.Uint16(ico[0:2]); got != 0 {
		t.Fatalf("reserved is %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(ico[2:4]); got != 1 {
		t.Fatalf("type is %d, want 1 (icon)", got)
	}
	if got := int(binary.LittleEndian.Uint16(ico[4:6])); got != len(sizes) {
		t.Fatalf("holds %d entries, want %d", got, len(sizes))
	}

	for i, size := range sizes {
		entry := ico[6+16*i : 6+16*(i+1)]
		if got := int(entry[0]); got != size%256 {
			t.Fatalf("entry %d declares width %d, want %d", i, got, size%256)
		}
		length := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if offset+length > len(ico) {
			t.Fatalf("entry %d runs past the end: %d+%d in %d bytes", i, offset, length, len(ico))
		}
		img, err := png.Decode(bytes.NewReader(ico[offset : offset+length]))
		if err != nil {
			t.Fatalf("entry %d does not decode: %v", i, err)
		}
		if img.Bounds() != image.Rect(0, 0, size, size) {
			t.Fatalf("entry %d is %v, want %dx%d", i, img.Bounds(), size, size)
		}
	}
}

func TestICORejectsSizesItCannotAverage(t *testing.T) {
	frames, err := Frames(trayIcon(t), 2)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if _, err := ICO(frames[0], 48); err == nil {
		t.Fatal("48 does not divide 64, want an error")
	}
	if _, err := ICO(frames[0]); err == nil {
		t.Fatal("want an error for an ICO with no sizes")
	}
}
