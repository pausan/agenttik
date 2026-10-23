package screen

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// startXvfb runs a private X server for one test and points DISPLAY at it.
// Xvfb picks a free display itself and writes its number to -displayfd, so
// tests running at once never share one.
func startXvfb(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb not installed")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("Xvfb", "-displayfd", "3", "-screen", "0", "640x480x24", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{w}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	number := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(r).ReadString('\n')
		number <- strings.TrimSpace(line)
	}()
	select {
	case n := <-number:
		if n == "" {
			t.Fatal("Xvfb did not start")
		}
		t.Setenv("DISPLAY", ":"+n)
		t.Setenv("WAYLAND_DISPLAY", "")
		return ":" + n
	case <-time.After(10 * time.Second):
		t.Fatal("Xvfb did not start")
		return ""
	}
}

func openTest(t *testing.T) *x11 {
	t.Helper()
	d, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d.(*x11)
}

// window maps a solid window on a separate client, selecting the events
// given, and waits until it is on screen.
func window(t *testing.T, conn *xgb.Conn, x, y, w, h int16, pixel uint32, events uint32) xproto.Window {
	t.Helper()
	scr := xproto.Setup(conn).DefaultScreen(conn)
	win, _ := xproto.NewWindowId(conn)
	err := xproto.CreateWindowChecked(conn, scr.RootDepth, win, scr.Root, x, y, uint16(w), uint16(h), 0,
		xproto.WindowClassInputOutput, scr.RootVisual,
		xproto.CwBackPixel|xproto.CwOverrideRedirect|xproto.CwEventMask, []uint32{pixel, 1, events | xproto.EventMaskExposure}).Check()
	if err != nil {
		t.Fatal(err)
	}
	xproto.MapWindow(conn, win)
	waitEvent(t, conn, func(ev xgb.Event) bool { _, ok := ev.(xproto.ExposeEvent); return ok })
	return win
}

func waitEvent(t *testing.T, conn *xgb.Conn, match func(xgb.Event) bool) xgb.Event {
	t.Helper()
	got := make(chan xgb.Event, 1)
	go func() {
		for {
			ev, err := conn.WaitForEvent()
			if ev == nil && err == nil {
				return
			}
			if ev != nil && match(ev) {
				got <- ev
				return
			}
		}
	}()
	select {
	case ev := <-got:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("event never came")
		return nil
	}
}

func testConn(t *testing.T) *xgb.Conn {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return conn
}

func TestX11GrabSeesWindowsAndCountsDamage(t *testing.T) {
	startXvfb(t)
	d := openTest(t)
	if w, h := d.Size(); w != 640 || h != 480 {
		t.Fatalf("size %dx%d", w, h)
	}
	if d.shmData == nil {
		t.Fatal("MIT-SHM not used on a local server")
	}
	before := d.Changes()
	window(t, testConn(t), 100, 50, 40, 30, 0xff0000, 0)
	deadline := time.Now().Add(2 * time.Second)
	for d.Changes() == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if d.Changes() == before {
		t.Fatal("damage not counted")
	}
	pix, w, _, err := d.Grab(nil)
	if err != nil {
		t.Fatal(err)
	}
	at := func(x, y int) [3]byte { i := (y*w + x) * 4; return [3]byte{pix[i+2], pix[i+1], pix[i]} }
	if got := at(110, 60); got != [3]byte{255, 0, 0} {
		t.Fatalf("inside the window: %v", got)
	}
	if got := at(10, 10); got == [3]byte{255, 0, 0} {
		t.Fatalf("outside the window is red too")
	}
}

func TestX11MouseAndKeys(t *testing.T) {
	startXvfb(t)
	d := openTest(t)
	conn := testConn(t)
	win := window(t, conn, 0, 0, 640, 480, 0xffffff, xproto.EventMaskKeyPress|xproto.EventMaskButtonPress)
	xproto.SetInputFocus(conn, xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime)
	xproto.GetInputFocus(conn).Reply()

	if err := d.Move(123, 45); err != nil {
		t.Fatal(err)
	}
	if x, y, _, err := d.Cursor(); err != nil || x != 123 || y != 45 {
		t.Fatalf("pointer at %d,%d (%v)", x, y, err)
	}
	d.Button(3, true)
	d.Button(3, false)
	ev := waitEvent(t, conn, func(ev xgb.Event) bool { _, ok := ev.(xproto.ButtonPressEvent); return ok }).(xproto.ButtonPressEvent)
	if ev.Detail != 3 || ev.EventX != 123 || ev.EventY != 45 {
		t.Fatalf("button %d at %d,%d", ev.Detail, ev.EventX, ev.EventY)
	}

	keysymOf := func(kc xproto.Keycode, level int) xproto.Keysym {
		setup := xproto.Setup(conn)
		m, err := xproto.GetKeyboardMapping(conn, kc, 1).Reply()
		if err != nil || int(m.KeysymsPerKeycode) <= level {
			t.Fatal(err)
		}
		_ = setup
		return m.Keysyms[level]
	}
	key := func() xproto.KeyPressEvent {
		return waitEvent(t, conn, func(ev xgb.Event) bool { _, ok := ev.(xproto.KeyPressEvent); return ok }).(xproto.KeyPressEvent)
	}

	// An upper-case letter holds Shift around its key.
	if err := d.Type('Q'); err != nil {
		t.Fatal(err)
	}
	shift := key()
	if keysymOf(shift.Detail, 0) != 0xffe1 {
		t.Fatalf("first key typed %#x, want Shift_L", keysymOf(shift.Detail, 0))
	}
	q := key()
	if keysymOf(q.Detail, 1) != 'Q' || q.State&xproto.ModMaskShift == 0 {
		t.Fatalf("typed keysym %#x with state %#x", keysymOf(q.Detail, 1), q.State)
	}

	// A character the US layout lacks goes on a spare key.
	if err := d.Type('é'); err != nil {
		t.Fatal(err)
	}
	e := key()
	if got := keysymOf(e.Detail, 0); got != 0xe9 {
		t.Fatalf("é typed keysym %#x", got)
	}

	// Ctrl+C as a viewer sends it: modifier down, key, both up.
	d.Key(0xffe3, true)
	d.Key('c', true)
	d.Key('c', false)
	d.Key(0xffe3, false)
	key() // Control
	c := key()
	if keysymOf(c.Detail, 0) != 'c' || c.State&xproto.ModMaskControl == 0 {
		t.Fatalf("ctrl+c: keysym %#x state %#x", keysymOf(c.Detail, 0), c.State)
	}
}

func TestX11ClipboardBetweenPrograms(t *testing.T) {
	startXvfb(t)
	a, b := openTest(t), openTest(t)
	changes, cancel := b.ClipboardChanges()
	defer cancel()

	if err := a.SetClipboard("héllo from a"); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-changes:
		if text != "héllo from a" {
			t.Fatalf("change announced %q", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no clipboard change announced")
	}
	if text, err := b.Clipboard(); err != nil || text != "héllo from a" {
		t.Fatalf("b read %q, %v", text, err)
	}

	// Taking it back: a loses ownership and reads b's text.
	if err := b.SetClipboard("now b"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		text, err := a.Clipboard()
		if err == nil && text == "now b" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a read %q, %v", text, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
