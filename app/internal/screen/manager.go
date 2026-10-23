package screen

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/screen/vpx"
)

// Manager is this machine's screen, shared by every viewer and agent. The
// display opens on first use, not at startup, and a failure is remembered
// for a few seconds so a machine without one is not asked on every request.
type Manager struct {
	open func() (Display, error)

	mu      sync.Mutex
	display Display
	err     error
	failed  time.Time
	viewers map[string]*Viewer
}

const retryOpen = 5 * time.Second

func NewManager(open func() (Display, error)) *Manager {
	return &Manager{open: open, viewers: map[string]*Viewer{}}
}

// Display opens the screen if it is not open yet.
func (m *Manager) Display() (Display, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.display != nil {
		return m.display, nil
	}
	if m.err != nil && time.Since(m.failed) < retryOpen {
		return nil, m.err
	}
	m.display, m.err = m.open()
	if m.err != nil {
		m.failed = time.Now()
		return nil, m.err
	}
	return m.display, nil
}

// Info is what the UI needs to decide whether to offer a Screen tab.
type Info struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	// Control says why input will not arrive though the screen shows, as
	// on a Mac that has not allowed it.
	Control string   `json:"control,omitempty"`
	Width   int      `json:"width,omitempty"`
	Height  int      `json:"height,omitempty"`
	OS      string   `json:"os"`
	Codecs  []string `json:"codecs"`
}

func (m *Manager) Info() Info {
	info := Info{OS: runtime.GOOS, Codecs: Codecs()}
	d, err := m.Display()
	if err != nil {
		info.Reason = err.Error()
		return info
	}
	info.Available = true
	info.Width, info.Height = d.Size()
	if c, ok := d.(interface{ ControlReason() string }); ok {
		info.Control = c.ControlReason()
	}
	return info
}

// Codecs lists what this build can send, best first. vp9-444 is VP9 with
// colour at full resolution (profile 1), for browsers that decode it.
func Codecs() []string {
	if vpx.Load() == nil {
		return []string{"vp9-444", "vp9", "vp8", "jpeg"}
	}
	return []string{"jpeg"}
}

// Close ends every viewer and lets go of the display.
func (m *Manager) Close() {
	m.mu.Lock()
	viewers := m.viewers
	m.viewers = map[string]*Viewer{}
	d := m.display
	m.display = nil
	m.mu.Unlock()
	for _, v := range viewers {
		v.Close()
	}
	if d != nil {
		d.Close()
	}
}

// Viewer is one Screen tab: a stream of frames encoded for it alone, the
// acknowledgements that pace it, and the keys and buttons it is holding.
type Viewer struct {
	ID     string
	m      *Manager
	d      Display
	codec  string
	out    chan Message
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}

	mu       sync.Mutex
	sent     uint64 // sequence number of the last frame sent
	acked    uint64
	inFlight []sentFrame
	ackedAt  time.Time
	rate     *rateControl
	received uint64
	wantKey  bool
	keys     map[string]uint32 // held keys by KeyboardEvent.code
	buttons  map[int]bool
}

type sentFrame struct {
	seq   uint64
	bytes int
	at    time.Time
}

// Message is one event on a viewer's stream.
type Message struct {
	Type string `json:"t"` // hello, frame, cursor, shape, clip

	Seq   uint64 `json:"seq,omitempty"`
	Codec string `json:"codec,omitempty"`
	Key   bool   `json:"key,omitempty"`
	// W and H are the frame's size; SW and SH the screen's, which input
	// coordinates are in.
	W     int    `json:"w,omitempty"`
	H     int    `json:"h,omitempty"`
	SW    int    `json:"sw,omitempty"`
	SH    int    `json:"sh,omitempty"`
	Data  string `json:"d,omitempty"` // base64
	Bands []Band `json:"bands,omitempty"`
	// Cap is the bitrate, in kbit/s, the stream is held to for this viewer's
	// link.
	Cap int `json:"cap,omitempty"`

	X    int    `json:"x,omitempty"`
	Y    int    `json:"y,omitempty"`
	HotX int    `json:"hx,omitempty"`
	HotY int    `json:"hy,omitempty"`
	Text string `json:"text,omitempty"`
	ID   string `json:"id,omitempty"`
}

// Band is one horizontal strip of a JPEG frame.
type Band struct {
	Y    int    `json:"y"`
	Data string `json:"d"`
}

// Pacing: a viewer gets frames only while it has acknowledged enough of the
// ones before. That keeps the latency of every proxy in between (the window,
// a remote profile, the tunnel) bounded by what is unacknowledged rather
// than by how much their buffers hold.
const (
	frameRate     = 30
	frameInterval = time.Second / frameRate
	ackTimeout    = 2 * time.Second
	// A screen still for this long is encoded once more at higher quality.
	settleDelay = 300 * time.Millisecond
)

// Watch starts a viewer. codec is the client's preference; the hello
// message says what it gets.
func (m *Manager) Watch(codec string) (*Viewer, error) {
	d, err := m.Display()
	if err != nil {
		return nil, err
	}
	switch codec {
	case "vp9-444", "vp8", "jpeg":
	default:
		codec = "vp9"
	}
	if codec != "jpeg" && vpx.Load() != nil {
		codec = "jpeg"
	}
	id := make([]byte, 8)
	rand.Read(id) //nolint:errcheck // crypto/rand does not fail
	ctx, cancel := context.WithCancel(context.Background())
	v := &Viewer{
		ID: hex.EncodeToString(id), m: m, d: d, codec: codec,
		out: make(chan Message, 4), cancel: cancel, done: make(chan struct{}),
		wake: make(chan struct{}, 1), wantKey: true, ackedAt: time.Now(), rate: newRateControl(time.Now()),
		keys: map[string]uint32{}, buttons: map[int]bool{},
	}
	m.mu.Lock()
	m.viewers[v.ID] = v
	m.mu.Unlock()
	go v.run(ctx)
	return v, nil
}

// Viewer finds a viewer by ID.
func (m *Manager) Viewer(id string) *Viewer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.viewers[id]
}

// Messages is the viewer's stream. It closes when the viewer does.
func (v *Viewer) Messages() <-chan Message { return v.out }

// Close stops the stream and lets go of whatever the viewer was holding, so a
// closed tab never leaves Ctrl down on the machine.
func (v *Viewer) Close() {
	v.cancel()
	<-v.done
	v.m.mu.Lock()
	delete(v.m.viewers, v.ID)
	v.m.mu.Unlock()
	v.mu.Lock()
	keys, buttons := v.keys, v.buttons
	v.keys, v.buttons = map[string]uint32{}, map[int]bool{}
	v.mu.Unlock()
	for _, ks := range keys {
		v.d.Key(ks, false) //nolint:errcheck
	}
	for b := range buttons {
		v.d.Button(b, false) //nolint:errcheck
	}
}

// Ack records that the viewer has drawn every frame up to seq.
func (v *Viewer) Ack(seq uint64) {
	v.mu.Lock()
	if seq > v.acked && seq <= v.sent {
		v.acked = seq
		v.ackedAt = time.Now()
		i := 0
		for i < len(v.inFlight) && v.inFlight[i].seq <= seq {
			i++
		}
		if i > 0 {
			v.rate.rtt(time.Since(v.inFlight[i-1].at))
		}
		v.inFlight = v.inFlight[i:]
	}
	v.mu.Unlock()
	v.poke()
}

// Received records when frame seq arrived on the viewer's clock, in
// milliseconds from any origin, and how many kbit/s it has been receiving.
// This is what the rate control reads the link from.
func (v *Viewer) Received(seq uint64, arrivedMs, recvKbps float64) {
	if math.IsNaN(arrivedMs) || math.Abs(arrivedMs) >= float64(math.MaxInt64)/float64(time.Millisecond) {
		return
	}
	if math.IsNaN(recvKbps) || math.IsInf(recvKbps, 0) || recvKbps < 0 {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if seq <= v.received {
		return
	}
	for _, f := range v.inFlight {
		if f.seq == seq {
			v.received = seq
			v.rate.received(f.at, time.Duration(arrivedMs*float64(time.Millisecond)), recvKbps, time.Now())
			return
		}
	}
}

// RequestKey asks for a frame that decodes on its own, after a decoder
// error or a dropped connection.
func (v *Viewer) RequestKey() {
	v.mu.Lock()
	v.wantKey = true
	v.mu.Unlock()
	v.poke()
}

func (v *Viewer) poke() {
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

// ready says whether another frame may go: the viewer is not too far behind,
// or has gone quiet long enough that its acknowledgements are assumed lost.
func (v *Viewer) ready() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.inFlight) == 0 {
		return true
	}
	if time.Since(v.ackedAt) > ackTimeout {
		v.inFlight = v.inFlight[:0]
		v.acked = v.sent
		v.ackedAt = time.Now()
		return true
	}
	bytes := 0
	for _, f := range v.inFlight {
		bytes += f.bytes
	}
	maxFrames, maxBytes := v.rate.window()
	return len(v.inFlight) < maxFrames && bytes < maxBytes
}

func (v *Viewer) send(ctx context.Context, msg Message) bool {
	select {
	case v.out <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

func (v *Viewer) run(ctx context.Context) {
	defer close(v.done)
	defer close(v.out)
	clips, stopClips := v.d.ClipboardChanges()
	defer stopClips()

	sw, sh := v.d.Size()
	if !v.send(ctx, Message{Type: "hello", ID: v.ID, Codec: v.codec, SW: sw, SH: sh}) {
		return
	}
	var (
		enc        frameEncoder
		buf, prev  []byte
		changes    = v.d.Changes() - 1
		cx, cy     = -1, -1
		serial     uint32
		shapeKnown bool
		last       time.Time // when the last frame was grabbed
		pacing     bool      // waiting out the frame interval
		refine     bool      // the last frame has not been refined yet
	)
	defer func() {
		if enc != nil {
			enc.Close()
		}
	}()
	// The pointer is polled; moving it does not damage the screen.
	pointer := time.NewTicker(frameInterval)
	defer pointer.Stop()
	pace := time.NewTimer(frameInterval)
	pace.Stop()
	defer pace.Stop()
	// What moved was encoded for motion. Once the screen stops, it is
	// encoded once more at higher quality, as remote desktops do.
	settle := time.NewTimer(settleDelay)
	settle.Stop()
	defer settle.Stop()
	for {
		// A frame is grabbed when the screen changes, not on a clock: a clock
		// at the same rate as an animation lands just before half its changes
		// and merges them into the next frame. Damage while a frame interval
		// is still running, or while the viewer is behind, waits for the
		// timer or the acknowledgement instead.
		var damaged <-chan struct{}
		if !pacing && v.ready() {
			damaged = v.d.ChangedSince(changes)
		}
		select {
		case <-ctx.Done():
			return
		case text := <-clips:
			if !v.send(ctx, Message{Type: "clip", Text: text}) {
				return
			}
			continue
		case <-pointer.C:
			if !v.sendPointer(ctx, &cx, &cy, &serial, &shapeKnown) {
				return
			}
			continue
		case <-settle.C:
			if !refine || v.d.Changes() != changes {
				continue
			}
			if !v.ready() {
				settle.Reset(settleDelay)
				continue
			}
			refine = false
			v.mu.Lock()
			kbps := int(v.rate.kbps)
			v.mu.Unlock()
			ms := int(time.Since(last).Milliseconds())
			last = time.Now()
			msg, size, err := enc.Refine(prev, ms, kbps)
			if err != nil || size == 0 {
				continue
			}
			msg.SW, msg.SH = enc.Size()[0], enc.Size()[1]
			if !v.sendFrame(ctx, msg, size, kbps) {
				return
			}
			continue
		case <-pace.C:
			pacing = false
		case <-damaged:
		case <-v.wake:
		}

		if !v.ready() {
			continue
		}
		v.mu.Lock()
		key := v.wantKey
		v.mu.Unlock()
		now := v.d.Changes()
		if now == changes && !key {
			continue
		}
		v.mu.Lock()
		interval, kbps := v.rate.interval(), int(v.rate.kbps)
		v.mu.Unlock()
		if wait := time.Until(last.Add(interval)); wait > 0 {
			pace.Reset(wait)
			pacing = true
			continue
		}
		changes = now
		pix, w, h, err := v.d.Grab(buf)
		if err != nil {
			continue
		}
		ms := 0
		if !last.IsZero() {
			ms = int(time.Since(last).Milliseconds())
		}
		last = time.Now()
		buf = pix
		if enc == nil || enc.Size() != [2]int{w, h} {
			if enc != nil {
				enc.Close()
			}
			if enc, err = newFrameEncoder(v.codec, w, h); err != nil {
				v.codec = "jpeg"
				enc, _ = newFrameEncoder("jpeg", w, h)
			}
			key, prev = true, nil
		}
		if !key && prev != nil && bytes.Equal(prev, pix) {
			continue
		}
		enc.SetKbps(kbps)
		msg, size, err := enc.Encode(pix, prev, ms, key)
		if err != nil {
			continue
		}
		// The two buffers swap: this screen is the next one's reference.
		buf, prev = prev, pix
		if size == 0 {
			continue
		}
		v.mu.Lock()
		v.wantKey = false
		v.mu.Unlock()
		msg.SW, msg.SH = w, h
		if !v.sendFrame(ctx, msg, size, kbps) {
			return
		}
		refine = true
		settle.Reset(settleDelay)
	}
}

// sendFrame numbers a frame and counts it in flight until acknowledged.
func (v *Viewer) sendFrame(ctx context.Context, msg Message, size, kbps int) bool {
	v.mu.Lock()
	v.sent++
	msg.Seq, msg.Codec, msg.Cap = v.sent, v.codec, kbps
	v.inFlight = append(v.inFlight, sentFrame{seq: v.sent, bytes: size, at: time.Now()})
	v.mu.Unlock()
	return v.send(ctx, msg)
}

// sendPointer tells the viewer where the pointer is and, when it changed,
// what it looks like. The viewer draws it, so moving it costs a few bytes
// rather than a frame.
func (v *Viewer) sendPointer(ctx context.Context, cx, cy *int, serial *uint32, known *bool) bool {
	x, y, s, err := v.d.Cursor()
	if err != nil {
		return true
	}
	if s != *serial || !*known {
		if shape, err := v.d.CursorShape(); err == nil {
			*serial, *known = shape.Serial, true
			if msg, ok := shapeMessage(shape); ok && !v.send(ctx, msg) {
				return false
			}
		}
	}
	if x != *cx || y != *cy {
		*cx, *cy = x, y
		return v.send(ctx, Message{Type: "cursor", X: x, Y: y})
	}
	return true
}

func shapeMessage(s CursorShape) (Message, bool) {
	if s.W == 0 || s.H == 0 {
		return Message{}, false
	}
	png, err := encodePNG(s.RGBA, s.W, s.H)
	if err != nil {
		return Message{}, false
	}
	return Message{Type: "shape", W: s.W, H: s.H, HotX: s.HotX, HotY: s.HotY, Data: base64.StdEncoding.EncodeToString(png)}, true
}

// frameEncoder turns successive screens into frame messages.
type frameEncoder interface {
	Size() [2]int
	// Encode compresses pix; prev is the screen the viewer last got, nil
	// when it has none. size is the payload's length, 0 for nothing to send.
	Encode(pix, prev []byte, ms int, key bool) (msg Message, size int, err error)
	// SetKbps holds the stream to about kbps; JPEG does it with quality.
	SetKbps(kbps int)
	// Refine encodes the last screen again at higher quality, within what
	// kbps allows; size 0 when the encoder has nothing to add.
	Refine(pix []byte, ms, kbps int) (msg Message, size int, err error)
	Close()
}

func newFrameEncoder(codec string, w, h int) (frameEncoder, error) {
	if codec == "jpeg" {
		return &jpegEncoder{w: w, h: h}, nil
	}
	c := vpx.VP9
	if codec == "vp8" {
		c = vpx.VP8
	}
	i444 := codec == "vp9-444"
	scale := 1
	if w > 2560 || h > 1600 {
		scale = 2
	}
	enc, err := vpx.NewEncoder(w, h, vpx.Options{Codec: c, Scale: scale, I444: i444})
	if err != nil {
		return nil, err
	}
	return &vpxEncoder{enc: enc, w: w, h: h, kbps: vpx.DefaultKbps}, nil
}

type vpxEncoder struct {
	enc  *vpx.Encoder
	w, h int
	kbps int
}

func (e *vpxEncoder) Size() [2]int { return [2]int{e.w, e.h} }

func (e *vpxEncoder) SetKbps(kbps int) {
	// Only a real change reconfigures the encoder.
	if abs(kbps-e.kbps)*20 > e.kbps {
		if e.enc.SetKbps(kbps) == nil {
			e.kbps = kbps
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (e *vpxEncoder) Close() { e.enc.Close() }

func (e *vpxEncoder) Encode(pix, _ []byte, ms int, key bool) (Message, int, error) {
	frame, isKey, err := e.enc.Encode(pix, e.w*4, ms, key)
	return e.message(frame, isKey, err)
}

func (e *vpxEncoder) Refine(pix []byte, ms, kbps int) (Message, int, error) {
	frame, err := e.enc.Refine(pix, e.w*4, ms, refineQ(kbps))
	return e.message(frame, false, err)
}

// refineQ is the quantizer a still screen is refined to: a whole 1080p
// screen just scrolled costs about 130 KB at 4, 40-60 KB at 10 and 45 KB at
// 16 on a 1 Mbit/s cap, so each stays near half a second of the link.
func refineQ(kbps int) int {
	switch {
	case kbps >= 4000:
		return 4
	case kbps >= 1500:
		return 10
	case kbps >= 600:
		return 16
	default:
		return 24
	}
}

func (e *vpxEncoder) message(frame []byte, isKey bool, err error) (Message, int, error) {
	if err != nil || len(frame) == 0 {
		return Message{}, 0, err
	}
	return Message{Type: "frame", Key: isKey, W: e.enc.Width, H: e.enc.Height,
		Data: base64.StdEncoding.EncodeToString(frame)}, len(frame), nil
}

// ErrNotViewing is the answer for input to a viewer that has gone.
var ErrNotViewing = errors.New("no such screen viewer")
