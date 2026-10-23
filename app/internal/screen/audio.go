package screen

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/screen/vpx"
)

// Audio is the sound this machine plays, as 48 kHz interleaved 16-bit
// stereo. Implementations are safe to Close while Read waits.
type Audio interface {
	// Read fills pcm, waiting until it can. It fails once capture has ended.
	Read(pcm []int16) error
	Close()
}

// errAudioClosed is what Read answers once Close has been called.
var errAudioClosed = errors.New("audio capture closed")

// audioBatch is how many 20 ms packets travel in one message: fewer messages
// for 20 ms more latency.
const audioBatch = 2

// SetAudio starts or stops sending this machine's sound to the viewer.
func (v *Viewer) SetAudio(on bool) {
	v.audioMu.Lock()
	if on && v.audioStop == nil && !v.audioEnded && v.m.openAudio != nil {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		v.audioStop, v.audioDone = cancel, done
		go func() {
			defer close(done)
			v.streamAudio(ctx)
		}()
	}
	v.audioMu.Unlock()
	if !on {
		v.stopAudio(false)
	}
}

// stopAudio ends the sound and waits for it; final keeps it from starting
// again, as the viewer's stream is about to close.
func (v *Viewer) stopAudio(final bool) {
	v.audioMu.Lock()
	stop, done := v.audioStop, v.audioDone
	v.audioStop, v.audioDone = nil, nil
	if final {
		v.audioEnded = true
	}
	v.audioMu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}

// streamAudio sends Opus packets until ctx ends or capture fails, in which
// case the viewer is told why. Silence is not sent: packets are numbered by
// the 20 ms they cover, so the viewer sees where it was left out.
func (v *Viewer) streamAudio(ctx context.Context) {
	fail := func(err error) {
		if ctx.Err() == nil {
			v.send(ctx, Message{Type: "audio", Text: err.Error()})
		}
	}
	a, err := v.m.openAudio()
	if err != nil {
		fail(err)
		return
	}
	enc, err := vpx.NewOpus()
	if err != nil {
		a.Close()
		fail(err)
		return
	}
	defer enc.Close()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		// Close is what wakes a Read waiting for sound.
		select {
		case <-ctx.Done():
		case <-stopped:
		}
		a.Close()
	}()

	pcm := make([]int16, 2*vpx.OpusFrame)
	var (
		n     uint64 // 20 ms periods read so far
		first uint64 // the first packet in batch
		batch []string
	)
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		ok := v.send(ctx, Message{Type: "audio", Seq: first, Packets: batch})
		batch = nil
		return ok
	}
	for {
		if err := a.Read(pcm); err != nil {
			if flush() {
				fail(err)
			}
			return
		}
		n++
		if silent(pcm) {
			if !flush() {
				return
			}
			continue
		}
		pkt, err := enc.Encode(pcm)
		if err != nil {
			fail(err)
			return
		}
		if len(batch) == 0 {
			first = n
		}
		batch = append(batch, base64.StdEncoding.EncodeToString(pkt))
		if len(batch) == audioBatch && !flush() {
			return
		}
	}
}

func silent(pcm []int16) bool {
	for _, s := range pcm {
		if s != 0 {
			return false
		}
	}
	return true
}

// pcmBuffer joins what a capture callback delivers into the frames Read
// asks for. It keeps at most a second: a reader that falls behind loses the
// oldest sound rather than hearing it late.
type pcmBuffer struct {
	mu     sync.Mutex
	buf    []int16
	err    error
	notify chan struct{}
}

const pcmKeep = 2 * 48000 // one second of stereo

func newPCMBuffer() *pcmBuffer { return &pcmBuffer{notify: make(chan struct{}, 1)} }

func (b *pcmBuffer) write(pcm []int16) {
	b.mu.Lock()
	b.buf = append(b.buf, pcm...)
	if over := len(b.buf) - pcmKeep; over > 0 {
		b.buf = b.buf[over+over%2:]
	}
	b.mu.Unlock()
	b.wake()
}

// fail ends the buffer: Read answers err once what is left is too short.
func (b *pcmBuffer) fail(err error) {
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
	b.wake()
}

func (b *pcmBuffer) wake() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// read fills pcm. check is called every second while it waits, for a
// capture that can end without saying so.
func (b *pcmBuffer) read(pcm []int16, check func() error) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		b.mu.Lock()
		if len(b.buf) >= len(pcm) {
			copy(pcm, b.buf)
			b.buf = b.buf[:copy(b.buf, b.buf[len(pcm):])]
			b.mu.Unlock()
			return nil
		}
		err := b.err
		b.mu.Unlock()
		if err != nil {
			return err
		}
		select {
		case <-b.notify:
		case <-tick.C:
			if check != nil {
				if err := check(); err != nil {
					b.fail(err)
				}
			}
		}
	}
}
