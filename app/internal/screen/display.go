// Package screen shows this machine's screen to a viewer and takes its mouse,
// keyboard and clipboard back: a Screen tab for people and an HTTP API for
// agents. See specs/096-screen.md.
package screen

import "errors"

// Display is one machine's screen and input devices. Implementations are
// safe for concurrent use.
type Display interface {
	// Size is the screen in pixels.
	Size() (w, h int)
	// Grab copies the screen as BGRA into dst, growing it as needed, and
	// answers the copy with its size. Rows are w*4 bytes apart.
	Grab(dst []byte) (pix []byte, w, h int, err error)
	// Changes counts damage seen so far: a viewer whose count has not moved
	// does not need to grab. A display that cannot tell counts every call.
	Changes() uint64
	// Cursor is the pointer's position and the serial of its shape, which
	// changes whenever the shape does.
	Cursor() (x, y int, serial uint32, err error)
	// CursorShape is the pointer's current image, as RGBA.
	CursorShape() (CursorShape, error)

	Move(x, y int) error
	// Button presses or releases a mouse button: 1 left, 2 middle, 3 right,
	// 4/5 wheel up/down, 6/7 wheel left/right.
	Button(b int, down bool) error
	// Key presses or releases the key that types keysym, as is: modifiers
	// held at the time apply. A keysym the keyboard lacks is mapped to a
	// spare key first.
	Key(keysym uint32, down bool) error
	// Type taps the keys for one character, holding or lifting Shift as the
	// keyboard needs, whatever modifiers are held.
	Type(r rune) error

	Clipboard() (string, error)
	SetClipboard(text string) error
	// ClipboardChanges sends the clipboard's text each time another program
	// takes it. Cancel stops the subscription.
	ClipboardChanges() (changes <-chan string, cancel func())

	Close()
}

// CursorShape is the pointer image and the pixel inside it that points.
type CursorShape struct {
	W, H, HotX, HotY int
	RGBA             []byte
	Serial           uint32
}

// ErrUnavailable wraps every reason there is no display to show.
var ErrUnavailable = errors.New("screen unavailable")

type unavailable struct{ reason string }

func (u unavailable) Error() string { return u.reason }
func (u unavailable) Unwrap() error { return ErrUnavailable }

// Unavailable is an error saying why this machine has no screen to share.
func Unavailable(reason string) error { return unavailable{reason} }
