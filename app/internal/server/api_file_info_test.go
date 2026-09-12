package server

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestImageInfo(t *testing.T) {
	bounds := image.Rect(0, 0, 320, 420)
	rgb := image.NewRGBA(bounds)
	for i := 3; i < len(rgb.Pix); i += 4 {
		rgb.Pix[i] = 255
	}
	for _, tc := range []struct {
		name string
		img  image.Image
		bits int
	}{
		{"rgb", rgb, 24}, {"rgba", image.NewNRGBA(bounds), 32},
		{"gray", image.NewGray(bounds), 8}, {"gray16", image.NewGray16(bounds), 16},
		{"rgba64", image.NewNRGBA64(bounds), 64},
		{"palette", image.NewPaletted(bounds, color.Palette{color.Black, color.White}), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := png.Encode(&b, tc.img); err != nil {
				t.Fatal(err)
			}
			assertImageInfo(t, b.Bytes(), imageInfo{320, 420, tc.bits})
			for n := 0; n < min(b.Len(), 40); n++ {
				readImageInfo(b.Bytes()[:n])
			}
		})
	}
	for _, img := range []image.Image{rgb, image.NewGray(bounds)} {
		var b bytes.Buffer
		if err := jpeg.Encode(&b, img, nil); err != nil {
			t.Fatal(err)
		}
		bits := 24
		if _, gray := img.(*image.Gray); gray {
			bits = 8
		}
		assertImageInfo(t, b.Bytes(), imageInfo{320, 420, bits})
	}
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(bounds, color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	assertImageInfo(t, b.Bytes(), imageInfo{320, 420, 1})
	if readImageInfo([]byte("not an image")) != nil {
		t.Fatal("text has image metadata")
	}
}

func assertImageInfo(t *testing.T, b []byte, want imageInfo) {
	t.Helper()
	got := readImageInfo(b)
	if got == nil || *got != want {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
}

func TestBitmapAndWebPInfo(t *testing.T) {
	bmp := make([]byte, 54)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[14:], 40)
	binary.LittleEndian.PutUint32(bmp[18:], 320)
	binary.LittleEndian.PutUint32(bmp[22:], 0xfffffe5c) // top-down: -420
	binary.LittleEndian.PutUint16(bmp[28:], 24)
	assertImageInfo(t, bmp, imageInfo{320, 420, 24})
	ico := make([]byte, 38)
	copy(ico, []byte{0, 0, 1, 0, 2, 0, 16, 16})
	binary.LittleEndian.PutUint16(ico[12:], 24)
	binary.LittleEndian.PutUint16(ico[28:], 32)
	assertImageInfo(t, ico, imageInfo{256, 256, 32})
	webp := make([]byte, 30)
	copy(webp, "RIFF")
	copy(webp[8:], "WEBPVP8X")
	webp[20] = 0x10
	copy(webp[24:], []byte{0x3f, 1, 0, 0xa3, 1, 0})
	assertImageInfo(t, webp, imageInfo{320, 420, 32})
	copy(webp[12:], "VP8L")
	webp[20] = 0x2f
	binary.LittleEndian.PutUint32(webp[21:], 319|419<<14|1<<28)
	assertImageInfo(t, webp, imageInfo{320, 420, 32})
	copy(webp[12:], "VP8 ")
	copy(webp[23:], []byte{0x9d, 1, 0x2a})
	binary.LittleEndian.PutUint16(webp[26:], 320)
	binary.LittleEndian.PutUint16(webp[28:], 420)
	assertImageInfo(t, webp, imageInfo{320, 420, 24})
	for _, header := range [][]byte{bmp, ico, webp} {
		for n := range len(header) {
			readImageInfo(header[:n])
		}
	}
}

func TestFileInfoEndpoint(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("metadata", dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID) + "/file-info?path="
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewNRGBA(image.Rect(0, 0, 320, 420))); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"empty.txt": {}, "unicode.txt": []byte("é🎨"), "photo.png": pngBytes.Bytes(), "broken.png": []byte("broken")} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
		got := decode[fileInfo](t, do(t, s, "GET", base+name, nil))
		if got.Size != int64(len(data)) {
			t.Fatalf("%s: size = %d", name, got.Size)
		}
		if name == "photo.png" {
			if got.Image == nil || *got.Image != (imageInfo{320, 420, 32}) {
				t.Fatalf("image = %+v", got.Image)
			}
		} else if got.Image != nil {
			t.Fatalf("%s has image metadata", name)
		}
	}
	// Text beyond the editor/preview limits still has an exact size.
	f, err := os.Create(filepath.Join(dir, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(32 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := decode[fileInfo](t, do(t, s, "GET", base+"large.bin", nil)); got.Size != 32<<20 {
		t.Fatal(got)
	}
	for _, path := range []string{"../outside", ".", "photo.png&rev=--help"} {
		resp := do(t, s, "GET", base+path, nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
	}
	// A commit uses its own blob, even after the working copy is replaced.
	for _, args := range [][]string{{"init"}, {"add", "photo.png", "unicode.txt"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "fixture"}} {
		if _, err := runGit(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	got := decode[fileInfo](t, do(t, s, "GET", base+"photo.png&rev=HEAD", nil))
	if got.Size != int64(pngBytes.Len()) || got.Image == nil || got.Image.Bits != 32 {
		t.Fatalf("revision info = %+v", got)
	}
}
