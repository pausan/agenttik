package vpx

import (
	"testing"
	"time"
)

// screenBGRA draws a desktop-like screen: light background, dark text-ish
// bars, shifted down by offset rows to imitate scrolling.
func screenBGRA(w, h, offset int) []byte {
	buf := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			v := byte(240)
			if ((y+offset)/6)%3 == 0 && (x/7)%5 != 0 && ((x*31+(y+offset)/18*17)%97) > 20 {
				v = 30
			}
			buf[i], buf[i+1], buf[i+2], buf[i+3] = v, v, v+10, 255
		}
	}
	return buf
}

func needLibrary(t testing.TB) {
	if err := Load(); err != nil {
		t.Skip(err)
	}
}

func TestEncodeKeyThenSmallDeltas(t *testing.T) {
	needLibrary(t)
	for _, codec := range []int{VP8, VP9} {
		enc, err := NewEncoder(640, 360, Options{Codec: codec})
		if err != nil {
			t.Fatal(err)
		}
		still := screenBGRA(640, 360, 0)
		first, key, err := enc.Encode(still, 640*4, 0, true)
		if err != nil || !key || len(first) == 0 {
			t.Fatalf("VP%d first frame: %d bytes, key %v, %v", codec, len(first), key, err)
		}
		// An unchanged screen costs next to nothing once the encoder has
		// settled on it.
		var last []byte
		for i := 0; i < 5; i++ {
			last, key, err = enc.Encode(still, 640*4, 33, false)
			if err != nil || key {
				t.Fatalf("VP%d repeat: key %v, %v", codec, key, err)
			}
		}
		if len(last) > len(first)/10 {
			t.Fatalf("VP%d unchanged frame is %d bytes against a %d-byte keyframe", codec, len(last), len(first))
		}
		again, key, err := enc.Encode(still, 640*4, 33, true)
		if err != nil || !key || len(again) == 0 {
			t.Fatalf("VP%d forced keyframe: key %v, %v", codec, key, err)
		}
		enc.Close()
	}
}

// A first frame comes 0 ms after nothing. It must still get a keyframe's
// worth of bits, or a still screen stays blurred until something moves.
func TestFirstKeyframeIsNotStarved(t *testing.T) {
	needLibrary(t)
	still := screenBGRA(1280, 720, 0)
	size := func(ms int) int {
		enc, err := NewEncoder(1280, 720, Options{Codec: VP9, Kbps: 2000})
		if err != nil {
			t.Fatal(err)
		}
		defer enc.Close()
		frame, _, err := enc.Encode(still, 1280*4, ms, true)
		if err != nil {
			t.Fatal(err)
		}
		return len(frame)
	}
	if first, settled := size(0), size(1000); first*10 < settled*9 {
		t.Fatalf("first keyframe %d bytes, %d with a second to cover", first, settled)
	}
}

func TestEncodeHalvesLargeScreens(t *testing.T) {
	needLibrary(t)
	enc, err := NewEncoder(1280, 720, Options{Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if enc.Width != 640 || enc.Height != 360 {
		t.Fatalf("encoded size %dx%d", enc.Width, enc.Height)
	}
	if _, key, err := enc.Encode(screenBGRA(1280, 720, 0), 1280*4, 0, true); err != nil || !key {
		t.Fatalf("key %v, %v", key, err)
	}
}

func TestChangeBitrateWhileEncoding(t *testing.T) {
	needLibrary(t)
	for _, codec := range []int{VP8, VP9} {
		enc, err := NewEncoder(640, 360, Options{Codec: codec})
		if err != nil {
			t.Fatal(err)
		}
		defer enc.Close()
		for i, kbps := range []int{2000, 500, 100, 8000} {
			if err := enc.SetKbps(kbps); err != nil {
				t.Fatalf("VP%d at %d kbps: %v", codec, kbps, err)
			}
			frame, key, err := enc.Encode(screenBGRA(640, 360, i*4), 640*4, 33, i == 0)
			if err != nil || len(frame) == 0 || key != (i == 0) {
				t.Fatalf("VP%d at %d kbps: %d bytes, key %v, %v", codec, kbps, len(frame), key, err)
			}
		}
	}
}

// BenchmarkScroll1080p is the worst common case: the whole screen moving.
func BenchmarkScroll1080p(b *testing.B) {
	needLibrary(b)
	for _, codec := range []int{VP8, VP9} {
		b.Run(map[int]string{VP8: "vp8", VP9: "vp9"}[codec], func(b *testing.B) {
			enc, err := NewEncoder(1920, 1080, Options{Codec: codec})
			if err != nil {
				b.Fatal(err)
			}
			defer enc.Close()
			frames := make([][]byte, 30)
			for i := range frames {
				frames[i] = screenBGRA(1920, 1080, i*4)
			}
			enc.Encode(frames[0], 1920*4, 0, true)
			b.ResetTimer()
			start, bytes := time.Now(), 0
			for i := 0; i < b.N; i++ {
				out, _, err := enc.Encode(frames[i%30], 1920*4, 33, false)
				if err != nil {
					b.Fatal(err)
				}
				bytes += len(out)
			}
			b.ReportMetric(float64(bytes)*8*30/float64(b.N)/1e6, "Mbps@30fps")
			_ = start
		})
	}
}
