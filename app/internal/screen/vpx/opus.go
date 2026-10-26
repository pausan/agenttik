package vpx

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// Opus compresses 48 kHz stereo sound, the screen's audio, with the libopus
// linked into the same library. It is not safe for concurrent use.
type Opus struct {
	handle uintptr
	out    []byte
}

const (
	// OpusFrame is the samples per channel in one packet: 20 ms.
	OpusFrame = 960
	// OpusKbps is the bitrate the encoder aims at; music and film sound
	// clean at it, and it is small beside a screen.
	OpusKbps = 64
)

// NewOpus opens an encoder.
func NewOpus() (*Opus, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := aopOpen(OpusKbps)
	if h == 0 {
		return nil, fmt.Errorf("open Opus encoder: %s", avxError())
	}
	return &Opus{handle: h, out: make([]byte, 1500)}, nil
}

// Encode compresses OpusFrame interleaved stereo samples into one packet,
// which stays valid until the next call.
func (o *Opus) Encode(pcm []int16) ([]byte, error) {
	if len(pcm) != 2*OpusFrame {
		return nil, errors.New("an Opus frame is 960 stereo samples")
	}
	n := aopEncode(o.handle, unsafe.Pointer(&pcm[0]), OpusFrame, unsafe.Pointer(&o.out[0]), int32(len(o.out)))
	runtime.KeepAlive(pcm)
	if n < 0 {
		return nil, fmt.Errorf("encode audio: %s", avxError())
	}
	return o.out[:n], nil
}

// Close frees the encoder.
func (o *Opus) Close() {
	if o.handle != 0 {
		aopClose(o.handle)
		o.handle = 0
	}
}
