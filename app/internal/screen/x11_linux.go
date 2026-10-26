package screen

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/damage"
	"github.com/jezek/xgb/shm"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
	"golang.org/x/sys/unix"
)

// x11 is an X server's screen: captured through MIT-SHM (or plain GetImage
// when the server is not on this machine), watched with DAMAGE, driven with
// XTEST, and its clipboard owned and read through an invisible window. All
// of it speaks the X protocol from Go; nothing is linked.
type x11 struct {
	// life is held for every request: xgb panics on a request sent after
	// Close, so Close waits for those in flight and later ones see closed.
	life   sync.RWMutex
	closed bool

	conn *xgb.Conn
	root xproto.Window
	win  xproto.Window // owns the clipboard and receives its replies

	mu      sync.Mutex // the capture buffer and the screen size
	w, h    int
	shmSeg  shm.Seg
	shmID   int
	shmData []byte

	changes  changeSignal
	damaged  bool // DAMAGE is working, so Changes can be trusted
	serial   atomic.Uint32
	maxBytes int // longest property this connection can write

	keyMu   sync.Mutex
	minKey  xproto.Keycode
	perKey  int
	keymap  []xproto.Keysym
	spares  []xproto.Keycode
	spare   int
	shifted map[xproto.Keycode]bool // Shift keys held through Key

	clipMu   sync.Mutex // one read of another program's clipboard at a time
	ownMu    sync.Mutex
	own      string // what this window holds while it owns the clipboard
	owning   bool
	replies  chan xproto.SelectionNotifyEvent
	subsMu   sync.Mutex
	subs     map[int]chan string
	nextSub  int
	atoms    struct{ clipboard, utf8, targets, text, prop, incr xproto.Atom }
	shiftKey xproto.Keycode
}

// Open connects to this machine's X display: $DISPLAY, or :0 for a server
// started outside the desktop session.
func Open() (Display, error) {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" && os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil, Unavailable("Wayland sessions are not supported yet; log in with an X11 session")
	}
	name := os.Getenv("DISPLAY")
	if name == "" {
		name = ":0"
	}
	conn, err := xgb.NewConnDisplay(name)
	if err != nil {
		return nil, Unavailable("no X display (" + name + "): " + err.Error())
	}
	d, err := openX11(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func openX11(conn *xgb.Conn) (*x11, error) {
	setup := xproto.Setup(conn)
	scr := setup.DefaultScreen(conn)
	if setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return nil, Unavailable("unsupported X image byte order")
	}
	bpp := 0
	for _, f := range setup.PixmapFormats {
		if f.Depth == scr.RootDepth {
			bpp = int(f.BitsPerPixel)
		}
	}
	if bpp != 32 || scr.RootDepth < 24 {
		return nil, Unavailable(fmt.Sprintf("unsupported X pixel format: depth %d, %d bits per pixel", scr.RootDepth, bpp))
	}
	if err := xtest.Init(conn); err != nil {
		return nil, Unavailable("the X server has no XTEST extension, which input needs")
	}
	if err := xfixes.Init(conn); err != nil {
		return nil, Unavailable("the X server has no XFIXES extension")
	}
	if _, err := xfixes.QueryVersion(conn, 5, 0).Reply(); err != nil {
		return nil, Unavailable("XFIXES: " + err.Error())
	}

	x := &x11{
		conn: conn, root: scr.Root, w: int(scr.WidthInPixels), h: int(scr.HeightInPixels),
		shmID: -1, replies: make(chan xproto.SelectionNotifyEvent, 1),
		subs: map[int]chan string{}, shifted: map[xproto.Keycode]bool{},
		maxBytes: int(setup.MaximumRequestLength)*4 - 64,
	}
	for name, atom := range map[string]*xproto.Atom{
		"CLIPBOARD": &x.atoms.clipboard, "UTF8_STRING": &x.atoms.utf8, "TARGETS": &x.atoms.targets,
		"TEXT": &x.atoms.text, "AGENTTIK_CLIPBOARD": &x.atoms.prop, "INCR": &x.atoms.incr,
	} {
		reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			return nil, err
		}
		*atom = reply.Atom
	}

	win, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	x.win = win
	if err := xproto.CreateWindowChecked(conn, 0, win, x.root, -10, -10, 1, 1, 0,
		xproto.WindowClassInputOnly, 0, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return nil, err
	}
	// The root's size changes with xrandr; its ConfigureNotify says when.
	xproto.ChangeWindowAttributes(conn, x.root, xproto.CwEventMask, []uint32{xproto.EventMaskStructureNotify})
	xfixes.SelectSelectionInput(conn, win, x.atoms.clipboard, xfixes.SelectionEventMaskSetSelectionOwner)
	xfixes.SelectCursorInput(conn, x.root, xfixes.CursorNotifyMaskDisplayCursor)

	if damage.Init(conn) == nil {
		if _, err := damage.QueryVersion(conn, 1, 1).Reply(); err == nil {
			if id, err := damage.NewDamageId(conn); err == nil {
				x.damaged = damage.CreateChecked(conn, id, xproto.Drawable(x.root), damage.ReportLevelNonEmpty).Check() == nil
			}
		}
	}
	if err := x.loadKeymap(); err != nil {
		return nil, err
	}
	x.attachShm()
	go x.events()
	return x, nil
}

// use holds the connection open for one call, or reports it closed.
func (x *x11) use() (func(), error) {
	x.life.RLock()
	if x.closed {
		x.life.RUnlock()
		return nil, errors.New("display closed")
	}
	return x.life.RUnlock, nil
}

func (x *x11) Size() (int, int) {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.w, x.h
}

func (x *x11) Changes() uint64 {
	if !x.damaged {
		return x.changes.count.Add(1)
	}
	return x.changes.count.Load()
}

func (x *x11) ChangedSince(n uint64) <-chan struct{} {
	if !x.damaged {
		return closed
	}
	return x.changes.since(n)
}

// attachShm shares a capture buffer with the X server, sized for the screen
// as it is now. A server elsewhere on the network cannot attach it, and
// Grab then asks for the pixels over the connection instead.
func (x *x11) attachShm() {
	x.detachShm()
	if shm.Init(x.conn) != nil {
		return
	}
	size := x.w * x.h * 4
	id, err := unix.SysvShmGet(unix.IPC_PRIVATE, size, unix.IPC_CREAT|0o600)
	if err != nil {
		return
	}
	data, err := unix.SysvShmAttach(id, 0, 0)
	// Marked for removal now: it goes once both sides detach, even if this
	// process dies.
	unix.SysvShmCtl(id, unix.IPC_RMID, nil) //nolint:errcheck
	if err != nil {
		return
	}
	seg, err := shm.NewSegId(x.conn)
	if err == nil {
		err = shm.AttachChecked(x.conn, seg, uint32(id), false).Check()
	}
	if err != nil {
		unix.SysvShmDetach(data) //nolint:errcheck
		return
	}
	x.shmSeg, x.shmID, x.shmData = seg, id, data
}

func (x *x11) detachShm() {
	if x.shmData == nil {
		return
	}
	shm.Detach(x.conn, x.shmSeg)
	unix.SysvShmDetach(x.shmData) //nolint:errcheck
	x.shmData, x.shmID = nil, -1
}

func (x *x11) Grab(dst []byte) ([]byte, int, int, error) {
	done, err := x.use()
	if err != nil {
		return nil, 0, 0, err
	}
	defer done()
	x.mu.Lock()
	defer x.mu.Unlock()
	w, h := x.w, x.h
	size := w * h * 4
	if cap(dst) < size {
		dst = make([]byte, size)
	}
	dst = dst[:size]
	if x.shmData != nil && len(x.shmData) >= size {
		if _, err := shm.GetImage(x.conn, xproto.Drawable(x.root), 0, 0, uint16(w), uint16(h),
			0xffffffff, xproto.ImageFormatZPixmap, x.shmSeg, 0).Reply(); err != nil {
			return nil, 0, 0, err
		}
		copy(dst, x.shmData[:size])
		return dst, w, h, nil
	}
	reply, err := xproto.GetImage(x.conn, xproto.ImageFormatZPixmap, xproto.Drawable(x.root),
		0, 0, uint16(w), uint16(h), 0xffffffff).Reply()
	if err != nil {
		return nil, 0, 0, err
	}
	if len(reply.Data) < size {
		return nil, 0, 0, errors.New("short screen image")
	}
	copy(dst, reply.Data[:size])
	return dst, w, h, nil
}

func (x *x11) Cursor() (int, int, uint32, error) {
	done, err := x.use()
	if err != nil {
		return 0, 0, 0, err
	}
	defer done()
	reply, err := xproto.QueryPointer(x.conn, x.root).Reply()
	if err != nil {
		return 0, 0, 0, err
	}
	return int(reply.RootX), int(reply.RootY), x.serial.Load(), nil
}

func (x *x11) CursorShape() (CursorShape, error) {
	done, err := x.use()
	if err != nil {
		return CursorShape{}, err
	}
	defer done()
	reply, err := xfixes.GetCursorImage(x.conn).Reply()
	if err != nil {
		return CursorShape{}, err
	}
	n := int(reply.Width) * int(reply.Height)
	rgba := make([]byte, n*4)
	for i := 0; i < n && i < len(reply.CursorImage); i++ {
		// Premultiplied ARGB, as XFIXES hands it out.
		p := reply.CursorImage[i]
		a := byte(p >> 24)
		r, g, b := byte(p>>16), byte(p>>8), byte(p)
		if a != 0 && a != 255 {
			r, g, b = byte(int(r)*255/int(a)), byte(int(g)*255/int(a)), byte(int(b)*255/int(a))
		}
		rgba[i*4], rgba[i*4+1], rgba[i*4+2], rgba[i*4+3] = r, g, b, a
	}
	x.serial.Store(reply.CursorSerial)
	return CursorShape{W: int(reply.Width), H: int(reply.Height), HotX: int(reply.Xhot), HotY: int(reply.Yhot), RGBA: rgba, Serial: reply.CursorSerial}, nil
}

func (x *x11) Move(px, py int) error {
	done, err := x.use()
	if err != nil {
		return err
	}
	defer done()
	return x.fake(xproto.MotionNotify, 0, px, py)
}

func (x *x11) Button(b int, down bool) error {
	done, err := x.use()
	if err != nil {
		return err
	}
	defer done()
	if b < 1 || b > 9 {
		return fmt.Errorf("no mouse button %d", b)
	}
	kind := byte(xproto.ButtonRelease)
	if down {
		kind = xproto.ButtonPress
	}
	return x.fake(kind, byte(b), 0, 0)
}

func (x *x11) fake(kind, detail byte, px, py int) error {
	return xtest.FakeInputChecked(x.conn, kind, detail, 0, x.root, int16(px), int16(py), 0).Check()
}

func (x *x11) Key(keysym uint32, down bool) error {
	done, err := x.use()
	if err != nil {
		return err
	}
	defer done()
	x.keyMu.Lock()
	defer x.keyMu.Unlock()
	kc, _, err := x.keycodeLocked(xproto.Keysym(keysym))
	if err != nil {
		return err
	}
	if keysym == 0xffe1 || keysym == 0xffe2 {
		if down {
			x.shifted[kc] = true
		} else {
			delete(x.shifted, kc)
		}
	}
	return x.press(kc, down)
}

func (x *x11) press(kc xproto.Keycode, down bool) error {
	kind := byte(xproto.KeyRelease)
	if down {
		kind = xproto.KeyPress
	}
	return x.fake(kind, byte(kc), 0, 0)
}

func (x *x11) Type(r rune) error {
	done, err := x.use()
	if err != nil {
		return err
	}
	defer done()
	x.keyMu.Lock()
	defer x.keyMu.Unlock()
	kc, level, err := x.keycodeLocked(xproto.Keysym(KeysymForRune(r)))
	if err != nil {
		return err
	}
	if reply, err := xproto.QueryPointer(x.conn, x.root).Reply(); err == nil &&
		reply.Mask&xproto.ModMaskLock != 0 && x.isLetterLocked(kc) {
		level ^= 1 // Caps Lock already swaps a letter's case
	}
	held := len(x.shifted) > 0
	switch {
	case level == 1 && !held:
		x.press(x.shiftKey, true)
		defer x.press(x.shiftKey, false)
	case level == 0 && held:
		for k := range x.shifted {
			x.press(k, false)
		}
		defer func() {
			for k := range x.shifted {
				x.press(k, true)
			}
		}()
	}
	if err := x.press(kc, true); err != nil {
		return err
	}
	return x.press(kc, false)
}

// isLetterLocked says whether the key's two levels are a letter's two cases,
// which is what Caps Lock affects.
func (x *x11) isLetterLocked(kc xproto.Keycode) bool {
	i := int(kc-x.minKey) * x.perKey
	if x.perKey < 2 || i+1 >= len(x.keymap) {
		return false
	}
	lo, up := x.keymap[i], x.keymap[i+1]
	return lo >= 'a' && lo <= 'z' && up == lo-32
}

// loadKeymap reads which keysyms each key types, and which keys type
// nothing and can be borrowed for a character the layout lacks.
func (x *x11) loadKeymap() error {
	setup := xproto.Setup(x.conn)
	count := byte(setup.MaxKeycode - setup.MinKeycode + 1)
	reply, err := xproto.GetKeyboardMapping(x.conn, setup.MinKeycode, count).Reply()
	if err != nil {
		return err
	}
	x.keyMu.Lock()
	defer x.keyMu.Unlock()
	x.minKey, x.perKey, x.keymap = setup.MinKeycode, int(reply.KeysymsPerKeycode), reply.Keysyms
	x.spares = x.spares[:0]
	for i := 0; i < int(count); i++ {
		empty := true
		for l := 0; l < x.perKey; l++ {
			if x.keymap[i*x.perKey+l] != 0 {
				empty = false
			}
		}
		if empty {
			x.spares = append(x.spares, x.minKey+xproto.Keycode(i))
		}
	}
	if kc, _, ok := x.findLocked(0xffe1); ok {
		x.shiftKey = kc
	}
	return nil
}

// findLocked is the key and level (0 plain, 1 shifted) that type ks.
func (x *x11) findLocked(ks xproto.Keysym) (xproto.Keycode, int, bool) {
	levels := min(2, x.perKey)
	for l := 0; l < levels; l++ {
		for i := 0; i*x.perKey+l < len(x.keymap); i++ {
			if x.keymap[i*x.perKey+l] == ks {
				return x.minKey + xproto.Keycode(i), l, true
			}
		}
	}
	// A key listing only the lower case types the upper case shifted.
	if ks >= 'A' && ks <= 'Z' {
		if kc, l, ok := x.findLocked(ks + 32); ok && l == 0 {
			return kc, 1, true
		}
	}
	return 0, 0, false
}

// keycodeLocked finds ks on the keyboard or puts it on a spare key, the way
// xdotool types characters the layout has no key for.
func (x *x11) keycodeLocked(ks xproto.Keysym) (xproto.Keycode, int, error) {
	if kc, level, ok := x.findLocked(ks); ok {
		return kc, level, nil
	}
	if len(x.spares) == 0 {
		return 0, 0, fmt.Errorf("no key for keysym %#x and no spare key to map it to", ks)
	}
	kc := x.spares[x.spare%len(x.spares)]
	x.spare++
	syms := make([]xproto.Keysym, x.perKey)
	syms[0] = ks
	if x.perKey > 1 {
		syms[1] = ks
	}
	if err := xproto.ChangeKeyboardMappingChecked(x.conn, 1, kc, byte(x.perKey), syms).Check(); err != nil {
		return 0, 0, err
	}
	copy(x.keymap[int(kc-x.minKey)*x.perKey:], syms)
	// Clients read the new mapping when MappingNotify reaches them; a round
	// trip gives them a moment before the key arrives.
	xproto.GetInputFocus(x.conn).Reply() //nolint:errcheck
	time.Sleep(5 * time.Millisecond)
	return kc, 0, nil
}

func (x *x11) SetClipboard(text string) error {
	done, err := x.use()
	if err != nil {
		return err
	}
	defer done()
	if len(text) > x.maxBytes {
		return fmt.Errorf("clipboard text is over %d KB", x.maxBytes/1024)
	}
	x.ownMu.Lock()
	x.own, x.owning = text, true
	x.ownMu.Unlock()
	if err := xproto.SetSelectionOwnerChecked(x.conn, x.win, x.atoms.clipboard, xproto.TimeCurrentTime).Check(); err != nil {
		return err
	}
	owner, err := xproto.GetSelectionOwner(x.conn, x.atoms.clipboard).Reply()
	if err != nil {
		return err
	}
	if owner.Owner != x.win {
		return errors.New("could not take the clipboard")
	}
	return nil
}

func (x *x11) Clipboard() (string, error) {
	done, err := x.use()
	if err != nil {
		return "", err
	}
	defer done()
	x.ownMu.Lock()
	if x.owning {
		text := x.own
		x.ownMu.Unlock()
		return text, nil
	}
	x.ownMu.Unlock()

	x.clipMu.Lock()
	defer x.clipMu.Unlock()
	owner, err := xproto.GetSelectionOwner(x.conn, x.atoms.clipboard).Reply()
	if err != nil || owner.Owner == 0 {
		return "", err
	}
	select { // a reply to an earlier read that timed out
	case <-x.replies:
	default:
	}
	xproto.ConvertSelection(x.conn, x.win, x.atoms.clipboard, x.atoms.utf8, x.atoms.prop, xproto.TimeCurrentTime)
	var ev xproto.SelectionNotifyEvent
	select {
	case ev = <-x.replies:
	case <-time.After(2 * time.Second):
		return "", errors.New("the clipboard's owner did not answer")
	}
	if ev.Property == 0 {
		return "", nil // it has no text
	}
	prop, err := xproto.GetProperty(x.conn, true, x.win, x.atoms.prop, xproto.GetPropertyTypeAny, 0, uint32(x.maxBytes/4)).Reply()
	if err != nil {
		return "", err
	}
	if prop.Type == x.atoms.incr {
		return "", errors.New("clipboard text too large to read")
	}
	return string(prop.Value), nil
}

func (x *x11) ClipboardChanges() (<-chan string, func()) {
	ch := make(chan string, 1)
	x.subsMu.Lock()
	id := x.nextSub
	x.nextSub++
	x.subs[id] = ch
	x.subsMu.Unlock()
	return ch, func() {
		x.subsMu.Lock()
		delete(x.subs, id)
		x.subsMu.Unlock()
	}
}

func (x *x11) publishClipboard() {
	text, err := x.Clipboard()
	if err != nil {
		return
	}
	x.subsMu.Lock()
	defer x.subsMu.Unlock()
	for _, ch := range x.subs {
		select { // only the latest text matters
		case <-ch:
		default:
		}
		ch <- text
	}
}

// events is the connection's one reader: damage, clipboard requests and
// replies, keymap and screen size changes.
func (x *x11) events() {
	for {
		ev, err := x.conn.WaitForEvent()
		if ev == nil && err == nil {
			return // closed
		}
		if err != nil {
			continue // a failed unchecked request; the caller saw nothing
		}
		x.handle(ev)
	}
}

func (x *x11) handle(ev xgb.Event) {
	done, err := x.use()
	if err != nil {
		return
	}
	defer done()
	switch e := ev.(type) {
	case damage.NotifyEvent:
		x.changes.bump()
		damage.Subtract(x.conn, e.Damage, 0, 0)
	case xfixes.CursorNotifyEvent:
		x.serial.Store(e.CursorSerial)
		x.changes.bump()
	case xproto.ConfigureNotifyEvent:
		if e.Window == x.root {
			x.mu.Lock()
			if int(e.Width) != x.w || int(e.Height) != x.h {
				x.w, x.h = int(e.Width), int(e.Height)
				x.attachShm()
			}
			x.mu.Unlock()
			x.changes.bump()
		}
	case xproto.MappingNotifyEvent:
		if e.Request == xproto.MappingKeyboard {
			if err := x.loadKeymap(); err != nil {
				log.Printf("screen: reload keymap: %v", err)
			}
		}
	case xproto.SelectionRequestEvent:
		x.answer(e)
	case xproto.SelectionClearEvent:
		x.ownMu.Lock()
		x.owning, x.own = false, ""
		x.ownMu.Unlock()
	case xproto.SelectionNotifyEvent:
		select {
		case x.replies <- e:
		default:
		}
	case xfixes.SelectionNotifyEvent:
		if e.Owner != x.win && e.Owner != 0 {
			go x.publishClipboard()
		}
	}
}

// answer hands this window's clipboard text to the program asking for it.
func (x *x11) answer(e xproto.SelectionRequestEvent) {
	x.ownMu.Lock()
	text, owning := x.own, x.owning
	x.ownMu.Unlock()
	prop := e.Property
	if prop == 0 {
		prop = e.Target // an obsolete requestor
	}
	switch {
	case !owning || e.Selection != x.atoms.clipboard:
		prop = 0
	case e.Target == x.atoms.targets:
		atoms := []xproto.Atom{x.atoms.targets, x.atoms.utf8, x.atoms.text, xproto.AtomString}
		buf := make([]byte, 4*len(atoms))
		for i, a := range atoms {
			binary.LittleEndian.PutUint32(buf[4*i:], uint32(a))
		}
		xproto.ChangeProperty(x.conn, xproto.PropModeReplace, e.Requestor, prop, xproto.AtomAtom, 32, uint32(len(atoms)), buf)
	case e.Target == x.atoms.utf8 || e.Target == x.atoms.text || e.Target == xproto.AtomString:
		kind := x.atoms.utf8
		if e.Target == xproto.AtomString {
			kind = xproto.AtomString
		}
		xproto.ChangeProperty(x.conn, xproto.PropModeReplace, e.Requestor, prop, kind, 8, uint32(len(text)), []byte(text))
	default:
		prop = 0
	}
	reply := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target, Property: prop}
	xproto.SendEvent(x.conn, false, e.Requestor, 0, string(reply.Bytes()))
}

func (x *x11) Close() {
	x.life.Lock()
	defer x.life.Unlock()
	if x.closed {
		return
	}
	x.mu.Lock()
	x.detachShm()
	x.mu.Unlock()
	x.closed = true
	x.conn.Close()
}
