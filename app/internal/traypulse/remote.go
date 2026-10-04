package traypulse

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
)

// RemoteIcon adds a Wi-Fi mark to the resting icon's bottom-right corner.
// It is rendered once at startup, like the local pulse frames.
func RemoteIcon(icon []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return nil, fmt.Errorf("traypulse: decode remote icon: %w", err)
	}
	out := image.NewNRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	scale := float64(out.Rect.Dx()) / 64
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			px, py := (float64(x)+0.5)/scale, (float64(y)+0.5)/scale
			plate := math.Max(0, math.Min(1, 15-math.Hypot(px-49, py-49)))
			if plate == 0 {
				continue
			}
			dx, dy := px-49, py-56
			distance := math.Hypot(dx, dy) - 2
			for _, radius := range []float64{9, 16} {
				angle := math.Atan2(dy, dx)
				d := math.Abs(math.Hypot(dx, dy) - radius)
				if angle < -3*math.Pi/4 || angle > -math.Pi/4 {
					end := radius / math.Sqrt2
					d = math.Min(math.Hypot(dx-end, dy+end), math.Hypot(dx+end, dy+end))
				}
				distance = math.Min(distance, d-1.25)
			}
			mark := math.Max(0, math.Min(1, 0.5-distance))
			p := out.PixOffset(x, y)
			for c, dark := range []float64{12, 18, 28} {
				base := float64(out.Pix[p+c])*(1-plate) + dark*plate
				out.Pix[p+c] = uint8(base*(1-mark) + 255*mark)
			}
			out.Pix[p+3] = uint8(float64(out.Pix[p+3])*(1-plate) + 255*plate)
		}
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, out); err != nil {
		return nil, fmt.Errorf("traypulse: encode remote icon: %w", err)
	}
	return buf.Bytes(), nil
}
