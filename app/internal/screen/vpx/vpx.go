// Package vpx encodes screens as VP8 or VP9 through libvpx, without cgo and
// without anything installed: the release binary embeds a shared library for
// its own platform (scripts/build-vpx.sh), writes it to the user cache once,
// and loads it with purego. A build without that library says so through
// Load, and the screen falls back to JPEG. See specs/096-screen.md.
package vpx

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Settings measured on 1080p desktop video, scrolling and typing: constant
// quality with a bitrate cap keeps a still screen almost free, VP9's screen
// tuning is about 30% smaller than VP8 at the same quality, and VP8 above
// speed 12 loses visible quality for little time. VP9 at 28 rather than 40
// keeps text sharp in motion for about 25% more bits and no more time.
const (
	VP8 = 8
	VP9 = 9

	DefaultKbps = 3000
	crfVP9      = 28
	crfVP8      = 30
	speedVP9    = 8
	speedVP8    = 12

	// keyMs is the least time a keyframe is said to cover. libvpx gives a
	// frame the bits its duration buys at the cap; a first frame of 1 ms
	// starved VP9 to 31 dB where 250 ms gives 51 dB at any cap.
	keyMs = 250

	shimABI = 2 // avx_abi in shim/agenttik_vpx.c
)

var (
	loadOnce sync.Once
	loadErr  error

	avxOpen   func(codec, w, h, scale, threads, kbps, crf, speed, i444 int32) uintptr
	avxEncode func(e uintptr, bgra unsafe.Pointer, stride, ms, forceKey, maxQ int32, out *unsafe.Pointer, key *int32) int32
	avxClose  func(e uintptr)
	avxKbps   func(e uintptr, kbps int32) int32
	avxError  func() string
)

// Load makes the library ready, once. Its error is the reason there is no
// VP8/VP9 on this build or machine.
func Load() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

func load() error {
	data, err := embedded.ReadFile(libDir + "/" + libName)
	if err != nil {
		return fmt.Errorf("no screen encoder in this build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	path, err := extract(data)
	if err != nil {
		return err
	}
	handle, err := openLibrary(path)
	if err != nil {
		return fmt.Errorf("load screen encoder: %w", err)
	}
	// A library built from another shim takes other arguments.
	var abi func() int32
	if sym, err := findSymbol(handle, "avx_abi"); err == nil {
		purego.RegisterFunc(&abi, sym)
	}
	if abi == nil || abi() != shimABI {
		return errors.New("screen encoder library is out of date; rebuild it with make vpx")
	}
	purego.RegisterLibFunc(&avxOpen, handle, "avx_open")
	purego.RegisterLibFunc(&avxEncode, handle, "avx_encode")
	purego.RegisterLibFunc(&avxClose, handle, "avx_close")
	purego.RegisterLibFunc(&avxKbps, handle, "avx_set_kbps")
	purego.RegisterLibFunc(&avxError, handle, "avx_error")
	return nil
}

// extract writes the library under the user cache, named by its hash, so a
// new build never loads an old copy and an unchanged one is written once.
func extract(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "agenttik")
	path := filepath.Join(dir, hex.EncodeToString(sum[:8])+"-"+libName)
	if existing, err := os.ReadFile(path); err == nil && sha256.Sum256(existing) == sum {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "vpx-*")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return "", errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

// Encoder turns screens of one size into one VP8 or VP9 stream. It is not
// safe for concurrent use.
type Encoder struct {
	handle uintptr
	Codec  int
	Width  int // encoded size
	Height int
	I444   bool
}

// Options shape an encoder. Zero values take the measured defaults.
type Options struct {
	Codec   int // VP8 or VP9
	Scale   int // 1, or 2 to halve each side
	Threads int
	Kbps    int // cap on the constant-quality bitrate
	// I444 keeps colour at full resolution (VP9 profile 1), so coloured text
	// stays sharp.
	I444 bool
}

// NewEncoder opens an encoder for w x h BGRA screens.
func NewEncoder(w, h int, opts Options) (*Encoder, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	if opts.Codec != VP8 {
		opts.Codec = VP9
	}
	if opts.Scale != 2 {
		opts.Scale = 1
	}
	if opts.Threads <= 0 {
		opts.Threads = min(4, max(1, runtime.NumCPU()/2))
	}
	if opts.Kbps <= 0 {
		opts.Kbps = DefaultKbps
	}
	crf, speed := crfVP9, speedVP9
	if opts.Codec == VP8 {
		crf, speed = crfVP8, speedVP8
	}
	i444 := int32(0)
	if opts.I444 {
		i444 = 1
	}
	h0 := avxOpen(int32(opts.Codec), int32(w), int32(h), int32(opts.Scale), int32(opts.Threads), int32(opts.Kbps), int32(crf), int32(speed), i444)
	if h0 == 0 {
		return nil, fmt.Errorf("open VP%d encoder: %s", opts.Codec, avxError())
	}
	return &Encoder{handle: h0, Codec: opts.Codec, Width: w / opts.Scale, Height: h / opts.Scale, I444: opts.I444}, nil
}

// Encode compresses one BGRA screen taken ms after the previous one. The
// returned bytes are a copy; nil means the encoder had nothing to say.
func (e *Encoder) Encode(bgra []byte, stride, ms int, forceKey bool) (frame []byte, key bool, err error) {
	return e.encode(bgra, stride, ms, forceKey, 0)
}

// Refine encodes the screen last given to Encode again, no worse than
// quantizer maxQ (0-63), sharpening what was blurred while it moved. Only
// VP9 can: VP8 refuses a new quantizer range mid-stream, and its static
// threshold skips blocks whose source did not change.
func (e *Encoder) Refine(bgra []byte, stride, ms, maxQ int) (frame []byte, err error) {
	if e.Codec != VP9 {
		return nil, nil
	}
	frame, _, err = e.encode(bgra, stride, ms, false, maxQ)
	return frame, err
}

func (e *Encoder) encode(bgra []byte, stride, ms int, forceKey bool, maxQ int) (frame []byte, key bool, err error) {
	if len(bgra) == 0 {
		return nil, false, errors.New("empty screen")
	}
	force := int32(0)
	if forceKey {
		force = 1
		ms = max(ms, keyMs)
	}
	var out unsafe.Pointer
	var isKey int32
	n := avxEncode(e.handle, unsafe.Pointer(&bgra[0]), int32(stride), int32(ms), force, int32(maxQ), &out, &isKey)
	runtime.KeepAlive(bgra)
	if n < 0 {
		return nil, false, fmt.Errorf("encode: %s", avxError())
	}
	if n == 0 {
		return nil, false, nil
	}
	frame = make([]byte, n)
	copy(frame, unsafe.Slice((*byte)(out), n))
	return frame, isKey != 0, nil
}

// SetKbps moves the bitrate cap; constant quality stays, so a screen that
// needs less than the cap is unaffected.
func (e *Encoder) SetKbps(kbps int) error {
	if avxKbps(e.handle, int32(kbps)) != 0 {
		return fmt.Errorf("set bitrate: %s", avxError())
	}
	return nil
}

// Close frees the encoder.
func (e *Encoder) Close() {
	if e.handle != 0 {
		avxClose(e.handle)
		e.handle = 0
	}
}
