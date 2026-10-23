package screen

import (
	"fmt"
	"sync"
)

// Fake is a Display in memory, for tests: a BGRA screen that changes when
// told to, and a log of the input it was sent.
type Fake struct {
	mu      sync.Mutex
	w, h    int
	pix     []byte
	changes changeSignal
	x, y    int
	log     []string
	clip    string
	subs    map[int]chan string
	nextSub int
}

func NewFake(w, h int) *Fake {
	f := &Fake{w: w, h: h, pix: make([]byte, w*h*4), subs: map[int]chan string{}}
	f.Fill(0, 0, w, h, 0x20, 0x20, 0x20)
	return f
}

// Fill paints a rectangle in r, g, b.
func (f *Fake) Fill(x, y, w, h int, r, g, b byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for py := y; py < min(y+h, f.h); py++ {
		for px := x; px < min(x+w, f.w); px++ {
			i := (py*f.w + px) * 4
			f.pix[i], f.pix[i+1], f.pix[i+2], f.pix[i+3] = b, g, r, 255
		}
	}
	f.changes.bump()
}

// Log is the input received so far, one line per call.
func (f *Fake) Log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

// Copy is another program putting text on the clipboard.
func (f *Fake) Copy(text string) {
	f.mu.Lock()
	f.clip = text
	subs := make([]chan string, 0, len(f.subs))
	for _, ch := range f.subs {
		subs = append(subs, ch)
	}
	f.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- text:
		default:
		}
	}
}

func (f *Fake) record(format string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, fmt.Sprintf(format, args...))
	return nil
}

func (f *Fake) Size() (int, int) { return f.w, f.h }

func (f *Fake) Grab(dst []byte) ([]byte, int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cap(dst) < len(f.pix) {
		dst = make([]byte, len(f.pix))
	}
	dst = dst[:len(f.pix)]
	copy(dst, f.pix)
	return dst, f.w, f.h, nil
}

func (f *Fake) Changes() uint64 { return f.changes.count.Load() }

func (f *Fake) ChangedSince(n uint64) <-chan struct{} { return f.changes.since(n) }

func (f *Fake) Cursor() (int, int, uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.x, f.y, 1, nil
}

func (f *Fake) CursorShape() (CursorShape, error) {
	return CursorShape{W: 2, H: 2, RGBA: make([]byte, 16), Serial: 1}, nil
}

func (f *Fake) Move(x, y int) error {
	f.mu.Lock()
	f.x, f.y = x, y
	f.mu.Unlock()
	return f.record("move %d %d", x, y)
}

func (f *Fake) Button(b int, down bool) error  { return f.record("button %d %v", b, down) }
func (f *Fake) Key(ks uint32, down bool) error { return f.record("key %#x %v", ks, down) }
func (f *Fake) Type(r rune) error              { return f.record("type %c", r) }

func (f *Fake) Clipboard() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clip, nil
}

func (f *Fake) SetClipboard(text string) error {
	f.mu.Lock()
	f.clip = text
	f.mu.Unlock()
	return f.record("clipboard %q", text)
}

func (f *Fake) ClipboardChanges() (<-chan string, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan string, 1)
	id := f.nextSub
	f.nextSub++
	f.subs[id] = ch
	return ch, func() {
		f.mu.Lock()
		delete(f.subs, id)
		f.mu.Unlock()
	}
}

func (f *Fake) Close() {}
