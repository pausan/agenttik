package traypulse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
)

// ICO wraps one PNG frame as a Windows icon holding the given square sizes.
//
// The shell reads PNG-compressed entries directly, which is what keeps this to
// a header and a directory: there is no BMP encoder here and no palette to
// build. Sizes must divide the frame's own size, so each one is an exact box
// average of it rather than a resample.
//
// The committed tray.ico still carries the resting icon and its full set of
// sizes; this is only for the frames, which are built while the app runs.
func ICO(frame []byte, sizes ...int) ([]byte, error) {
	if len(sizes) == 0 || len(sizes) > 255 {
		return nil, fmt.Errorf("traypulse: an ICO needs 1 to 255 sizes, got %d", len(sizes))
	}
	src, err := png.Decode(bytes.NewReader(frame))
	if err != nil {
		return nil, fmt.Errorf("traypulse: decode frame: %w", err)
	}
	rgba := toRGBA(src)

	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		if size < 1 || size > 256 || rgba.Rect.Dx()%size != 0 {
			return nil, fmt.Errorf("traypulse: %dpx does not divide the %dpx frame", size, rgba.Rect.Dx())
		}
		buf := new(bytes.Buffer)
		if err := png.Encode(buf, shrink(rgba, rgba.Rect.Dx()/size)); err != nil {
			return nil, fmt.Errorf("traypulse: encode %dpx entry: %w", size, err)
		}
		images[i] = buf.Bytes()
	}

	const headerLen, entryLen = 6, 16
	out := new(bytes.Buffer)
	binary.Write(out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(out, binary.LittleEndian, uint16(1)) // 1 = icon
	binary.Write(out, binary.LittleEndian, uint16(len(sizes)))
	offset := headerLen + entryLen*len(sizes)
	for i, size := range sizes {
		// 256 is written as 0; the field is one byte.
		out.WriteByte(byte(size % 256))
		out.WriteByte(byte(size % 256))
		out.WriteByte(0)                                   // palette size, 0 for truecolour
		out.WriteByte(0)                                   // reserved
		binary.Write(out, binary.LittleEndian, uint16(1))  // colour planes
		binary.Write(out, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(out, binary.LittleEndian, uint32(len(images[i])))
		binary.Write(out, binary.LittleEndian, uint32(offset))
		offset += len(images[i])
	}
	for _, img := range images {
		out.Write(img)
	}
	return out.Bytes(), nil
}

// shrink box-averages by an integer factor, which is exact for the 64px icon's
// 32 and 16px entries and avoids pulling in a resampling dependency.
func shrink(src *image.RGBA, factor int) *image.RGBA {
	if factor == 1 {
		return src
	}
	w, h := src.Rect.Dx()/factor, src.Rect.Dy()/factor
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	n := uint32(factor * factor)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					p := src.PixOffset(x*factor+dx, y*factor+dy)
					r += uint32(src.Pix[p])
					g += uint32(src.Pix[p+1])
					b += uint32(src.Pix[p+2])
					a += uint32(src.Pix[p+3])
				}
			}
			q := out.PixOffset(x, y)
			out.Pix[q] = uint8(r / n)
			out.Pix[q+1] = uint8(g / n)
			out.Pix[q+2] = uint8(b / n)
			out.Pix[q+3] = uint8(a / n)
		}
	}
	return out
}
