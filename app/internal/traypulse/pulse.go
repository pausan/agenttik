// Package traypulse animates the desktop tray icon while turns are running.
//
// The frames are derived from the tray icon itself rather than shipped beside
// it, so the logo stays a single asset: change web/public/agenttik.svg and its
// PNG export and the pulse follows. Deriving them costs one PNG decode and a
// few milliseconds of arithmetic, once, when the tray starts.
package traypulse

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Green is the logo's mark colour, used for the glow the pulse adds around it.
const (
	glowR, glowG, glowB = 0x00, 0xdc, 0x82
)

// The mark is the bright part of the icon: the green sparkle and the white
// spark. Everything below markLo is plate or the glow already baked into the
// export, and is left to the blur instead. Luma, not green, because the small
// spark is white.
const (
	markLo = 0.30
	markHi = 0.50
	// glowLo is where the blurred halo picks pixels up. It is lower than
	// markLo so the mark's own baked glow feeds the pulse's glow.
	glowLo = 0.22
)

// dim and bright are what the mark's brightness moves between. The resting
// icon is 1.0, so the pulse dips a little below it and rises well above: a
// tray icon caught at any point in the cycle should not look like the idle
// one.
const (
	dim    = 0.85
	bright = 1.30
)

// glowStrength is how much green the peak adds around the mark, as a fraction
// of full intensity. Enough to read at 22px without turning the plate green.
const glowStrength = 0.55

// glowRadius is the box blur's half-width, in pixels of the source icon.
const glowRadius = 3

// Frames renders the rising half of the pulse, from its dimmest at index 0 to
// its brightest at the last index. The animator plays it forwards and then
// backwards, so a cycle is 2*(count-1) steps and only these are held.
//
// icon is the tray PNG; the frames come back PNG-encoded at the same size.
func Frames(icon []byte, count int) ([][]byte, error) {
	if count < 2 {
		return nil, fmt.Errorf("traypulse: need at least 2 frames, got %d", count)
	}
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return nil, fmt.Errorf("traypulse: decode icon: %w", err)
	}
	base := toRGBA(src)
	w, h := base.Rect.Dx(), base.Rect.Dy()
	mark, glow := masks(base)
	glow = blur(glow, w, h, glowRadius)

	frames := make([][]byte, count)
	for i := range frames {
		// A raised cosine, so the pulse eases at both ends instead of
		// running at a constant rate into a corner.
		k := (1 - math.Cos(math.Pi*float64(i)/float64(count-1))) / 2
		buf := new(bytes.Buffer)
		if err := png.Encode(buf, render(base, mark, glow, k)); err != nil {
			return nil, fmt.Errorf("traypulse: encode frame %d: %w", i, err)
		}
		frames[i] = buf.Bytes()
	}
	return frames, nil
}

// render applies one point of the pulse: the mark changes brightness, and a
// green halo grows around it. k is 0 at the dimmest and 1 at the brightest.
func render(base *image.RGBA, mark, glow []float32, k float64) *image.RGBA {
	w, h := base.Rect.Dx(), base.Rect.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	level := float32(dim + (bright-dim)*k)
	halo := float32(glowStrength * k)

	for i := 0; i < w*h; i++ {
		p := base.Pix[i*4 : i*4+4 : i*4+4]
		// The export is not premultiplied in practice — the plate is opaque
		// and the mark sits on it — so the channels are scaled directly.
		gain := 1 + (level-1)*mark[i]
		r := float32(p[0]) * gain
		g := float32(p[1]) * gain
		b := float32(p[2]) * gain

		// Screen blend, so the halo lightens the plate without washing out
		// the mark it surrounds.
		if a := halo * glow[i]; a > 0 {
			r = screen(r, glowR*a)
			g = screen(g, glowG*a)
			b = screen(b, glowB*a)
		}

		q := out.Pix[i*4 : i*4+4 : i*4+4]
		q[0], q[1], q[2], q[3] = clamp(r), clamp(g), clamp(b), p[3]
	}
	return out
}

func screen(base, add float32) float32 {
	return 255 - (255-base)*(255-add)/255
}

// masks returns, per pixel, how much of the mark is there and how much feeds
// the glow. Both are ramps rather than thresholds, so the antialiased edge of
// the sparkle does not turn into a staircase when it brightens.
func masks(img *image.RGBA) (mark, glow []float32) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	mark = make([]float32, w*h)
	glow = make([]float32, w*h)
	for i := 0; i < w*h; i++ {
		p := img.Pix[i*4 : i*4+4 : i*4+4]
		y := luma(p[0], p[1], p[2]) * float32(p[3]) / 255
		mark[i] = ramp(y, markLo, markHi)
		glow[i] = ramp(y, glowLo, markHi)
	}
	return mark, glow
}

func luma(r, g, b uint8) float32 {
	return (0.299*float32(r) + 0.587*float32(g) + 0.114*float32(b)) / 255
}

// ramp is a smoothstep between lo and hi.
func ramp(v, lo, hi float32) float32 {
	if v <= lo {
		return 0
	}
	if v >= hi {
		return 1
	}
	t := (v - lo) / (hi - lo)
	return t * t * (3 - 2*t)
}

// blur is a separable box blur, run twice. Two boxes approximate a Gaussian
// closely enough for a halo and cost two passes per axis over 4096 pixels.
func blur(src []float32, w, h, radius int) []float32 {
	out := src
	for pass := 0; pass < 2; pass++ {
		out = blurAxis(out, w, h, radius, true)
		out = blurAxis(out, w, h, radius, false)
	}
	return out
}

func blurAxis(src []float32, w, h, radius int, horizontal bool) []float32 {
	out := make([]float32, len(src))
	outer, inner, stride := h, w, 1
	if !horizontal {
		outer, inner, stride = w, h, w
	}
	for o := 0; o < outer; o++ {
		start := o * w
		if !horizontal {
			start = o
		}
		for i := 0; i < inner; i++ {
			var sum float32
			var n int
			for d := -radius; d <= radius; d++ {
				j := i + d
				if j < 0 || j >= inner {
					continue
				}
				sum += src[start+j*stride]
				n++
			}
			out[start+i*stride] = sum / float32(n)
		}
	}
	return out
}

func clamp(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func toRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, color.RGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return out
}
