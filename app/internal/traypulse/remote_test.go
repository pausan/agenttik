package traypulse

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestRemoteIconBadgesOnlyBottomRight(t *testing.T) {
	base := trayIcon(t)
	icon, err := RemoteIcon(base)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := png.Decode(bytes.NewReader(base))
	after, err := png.Decode(bytes.NewReader(icon))
	if err != nil || after.Bounds() != before.Bounds() {
		t.Fatalf("badged icon bounds: %v", err)
	}
	before, after = toRGBA(before), toRGBA(after)
	changed, white := 0, 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if before.At(x, y) == after.At(x, y) {
				continue
			}
			if x < 34 || y < 34 {
				t.Fatalf("badge changed pixel outside bottom right: %d,%d", x, y)
			}
			changed++
			r, g, b, _ := after.At(x, y).RGBA()
			if r > 0xd000 && g > 0xd000 && b > 0xd000 {
				white++
			}
		}
	}
	if changed < 300 || white < 50 {
		t.Fatalf("missing Wi-Fi mark: %d changed pixels, %d white pixels", changed, white)
	}
	ico, err := ICO(icon, 16, 32, 64)
	if err != nil {
		t.Fatal(err)
	}
	for i, size := range []int{16, 32, 64} {
		entry := ico[6+16*i : 6+16*(i+1)]
		length, offset := binary.LittleEndian.Uint32(entry[8:12]), binary.LittleEndian.Uint32(entry[12:16])
		img, err := png.Decode(bytes.NewReader(ico[offset : offset+length]))
		if err != nil || img.Bounds().Dx() != size {
			t.Fatalf("invalid %dpx Windows icon: %v", size, err)
		}
	}
}
