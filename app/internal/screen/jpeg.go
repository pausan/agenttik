package screen

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"runtime"
	"sync"
)

// jpegEncoder is the fallback for a build without the VP8/VP9 library or a
// browser without WebCodecs: the screen in horizontal bands, and only the
// bands that changed, each a JPEG encoded on its own core. Typing touches a
// band or two; scrolling sends them all.
type jpegEncoder struct {
	w, h int
}

const (
	bandRows    = 64
	jpegQuality = 75
)

func (e *jpegEncoder) Size() [2]int { return [2]int{e.w, e.h} }
func (e *jpegEncoder) Close()       {}

func (e *jpegEncoder) Encode(pix, prev []byte, _ int, key bool) (Message, int, error) {
	stride := e.w * 4
	var changed []int
	for y := 0; y < e.h; y += bandRows {
		end := min(y+bandRows, e.h)
		if key || prev == nil || !bytes.Equal(pix[y*stride:end*stride], prev[y*stride:end*stride]) {
			changed = append(changed, y)
		}
	}
	if len(changed) == 0 {
		return Message{}, 0, nil
	}
	bands := make([]Band, len(changed))
	errs := make([]error, len(changed))
	sizes := make([]int, len(changed))
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), len(changed)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				y := changed[i]
				data, err := encodeJPEG(pix, e.w, y, min(y+bandRows, e.h), jpegQuality)
				bands[i], errs[i], sizes[i] = Band{Y: y, Data: base64.StdEncoding.EncodeToString(data)}, err, len(data)
			}
		}()
	}
	for i := range changed {
		work <- i
	}
	close(work)
	wg.Wait()
	size := 0
	for i, err := range errs {
		if err != nil {
			return Message{}, 0, err
		}
		size += sizes[i]
	}
	return Message{Type: "frame", Key: key || prev == nil, W: e.w, H: e.h, Bands: bands}, size, nil
}

// rgba copies rows y0..y1 of a BGRA screen into an opaque RGBA image.
func rgba(pix []byte, w, y0, y1 int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, y1-y0))
	src := pix[y0*w*4 : y1*w*4]
	dst := img.Pix
	for i := 0; i+3 < len(src); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 255
	}
	return img
}

func encodeJPEG(pix []byte, w, y0, y1, quality int) ([]byte, error) {
	var out bytes.Buffer
	err := jpeg.Encode(&out, rgba(pix, w, y0, y1), &jpeg.Options{Quality: quality})
	return out.Bytes(), err
}

// encodePNG writes straight RGBA pixels as a PNG.
func encodePNG(pixRGBA []byte, w, h int) ([]byte, error) {
	img := &image.NRGBA{Pix: pixRGBA, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	err := enc.Encode(&out, img)
	return out.Bytes(), err
}
