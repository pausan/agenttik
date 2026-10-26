package screen

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// gdi is the Windows primary monitor: captured with BitBlt into a DIB
// section, driven with SendInput, and its clipboard read and set as
// CF_UNICODETEXT. GDI cannot tell when the screen changes, so every grab
// counts as a change and the viewer compares frames itself.
//
// The process is made per-monitor DPI aware first, so sizes, the pointer
// and the picture are all in real pixels.
type gdi struct {
	mu     sync.Mutex
	closed bool
	w, h   int
	screen windows.Handle // the screen's DC
	mem    windows.Handle // a memory DC holding the DIB
	dib    windows.Handle
	bits   unsafe.Pointer
	old    windows.Handle // the memory DC's bitmap before the DIB

	changes changeSignal

	curMu  sync.Mutex
	cursor uintptr // the shape last read
	shape  CursorShape

	inMu    sync.Mutex
	shifted map[uint32]bool // Shift keysyms held through Key

	clip clipWatch
}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetCursorInfo          = user32.NewProc("GetCursorInfo")
	procGetIconInfo            = user32.NewProc("GetIconInfo")
	procDrawIconEx             = user32.NewProc("DrawIconEx")
	procSendInput              = user32.NewProc("SendInput")
	procVkKeyScanW             = user32.NewProc("VkKeyScanW")
	procMapVirtualKeyW         = user32.NewProc("MapVirtualKeyW")
	procGetKeyState            = user32.NewProc("GetKeyState")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procClipboardSequence      = user32.NewProc("GetClipboardSequenceNumber")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procGetObjectW             = gdi32.NewProc("GetObjectW")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGdiFlush               = gdi32.NewProc("GdiFlush")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
	procGlobalFree             = kernel32.NewProc("GlobalFree")
	procGlobalLock             = kernel32.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32.NewProc("GlobalUnlock")
)

const (
	smCxScreen         = 0
	smCyScreen         = 1
	srcCopy            = 0x00CC0020
	cfUnicodeText      = 13
	gmemMoveable       = 0x0002
	diNormal           = 0x0003
	inputMouse         = 0
	inputKeyboard      = 1
	mouseMove          = 0x0001
	mouseAbsolute      = 0x8000
	mouseWheel         = 0x0800
	mouseHWheel        = 0x1000
	wheelDelta         = 120
	keyExtended        = 0x0001
	keyUp              = 0x0002
	keyUnicode         = 0x0004
	vkShift            = 0x10
	vkCapital          = 0x14
	cursorShowing      = 0x00000001
	perMonitorAwareV2  = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, -4
	maxClipboardTries  = 10
	clipboardRetryWait = 10 * time.Millisecond
)

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmap struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitsPixel               uint16
	Bits                            uintptr
}

type cursorInfo struct {
	Size   uint32
	Flags  uint32
	Cursor uintptr
	X, Y   int32
}

type iconInfo struct {
	Icon        int32
	HotX, HotY  uint32
	Mask, Color windows.Handle
}

// mouseInput and keyInput are INPUT as SendInput takes it on 64-bit
// Windows: a type, then the union at offset 8, 40 bytes in all.
type mouseInput struct {
	Type      uint32
	_         uint32
	DX, DY    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

type keyInput struct {
	Type  uint32
	_     uint32
	VK    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	Extra uintptr
	_     [8]byte
}

// A wrong layout fails the build rather than every SendInput.
var (
	_ [unsafe.Sizeof(mouseInput{}) - 40]byte
	_ [40 - unsafe.Sizeof(mouseInput{})]byte
	_ [unsafe.Sizeof(keyInput{}) - 40]byte
	_ [40 - unsafe.Sizeof(keyInput{})]byte
	_ [unsafe.Offsetof(mouseInput{}.DX) - 8]byte
	_ [8 - unsafe.Offsetof(keyInput{}.VK)]byte
	_ [unsafe.Sizeof(cursorInfo{}) - 24]byte
	_ [unsafe.Sizeof(bitmapInfoHeader{}) - 40]byte
)

// Open captures the primary monitor of the desktop this process runs on.
func Open() (Display, error) {
	if err := procGetSystemMetrics.Find(); err != nil {
		return nil, Unavailable("no Windows desktop: " + err.Error())
	}
	// Fails harmlessly when the manifest or Wails already chose.
	procSetDpiAwarenessContext.Call(perMonitorAwareV2) //nolint:errcheck
	g := &gdi{shifted: map[uint32]bool{}}
	g.clip = clipWatch{seq: clipboardSequence, read: g.Clipboard}
	g.mu.Lock()
	err := g.resizeLocked()
	g.mu.Unlock()
	if err != nil {
		g.Close()
		return nil, err
	}
	return g, nil
}

func metric(i uintptr) int {
	n, _, _ := procGetSystemMetrics.Call(i)
	return int(int32(n))
}

// resizeLocked makes the capture DIB match the screen's size.
func (g *gdi) resizeLocked() error {
	w, h := metric(smCxScreen), metric(smCyScreen)
	if w <= 0 || h <= 0 {
		return Unavailable("no screen on this desktop")
	}
	if g.dib != 0 && w == g.w && h == g.h {
		return nil
	}
	g.freeLocked()
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return Unavailable("no screen on this desktop; a service cannot see one")
	}
	g.screen = windows.Handle(screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return errors.New("CreateCompatibleDC failed")
	}
	g.mem = windows.Handle(mem)
	// Top-down 32-bit BGRX: the layout Grab answers in.
	hdr := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	var bits unsafe.Pointer
	dib, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&hdr)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		return errors.New("CreateDIBSection failed")
	}
	g.dib, g.bits, g.w, g.h = windows.Handle(dib), bits, w, h
	old, _, _ := procSelectObject.Call(mem, dib)
	g.old = windows.Handle(old)
	return nil
}

func (g *gdi) freeLocked() {
	if g.mem != 0 {
		if g.old != 0 {
			procSelectObject.Call(uintptr(g.mem), uintptr(g.old)) //nolint:errcheck
		}
		procDeleteDC.Call(uintptr(g.mem)) //nolint:errcheck
	}
	if g.dib != 0 {
		procDeleteObject.Call(uintptr(g.dib)) //nolint:errcheck
	}
	if g.screen != 0 {
		procReleaseDC.Call(0, uintptr(g.screen)) //nolint:errcheck
	}
	g.mem, g.dib, g.old, g.screen, g.bits = 0, 0, 0, 0, nil
}

func (g *gdi) Size() (int, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.w, g.h
}

func (g *gdi) Changes() uint64 { return g.changes.count.Add(1) }

func (g *gdi) ChangedSince(uint64) <-chan struct{} { return closed }

func (g *gdi) Grab(dst []byte) ([]byte, int, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, 0, 0, errors.New("display closed")
	}
	if err := g.resizeLocked(); err != nil {
		return nil, 0, 0, err
	}
	w, h := g.w, g.h
	// Without CAPTUREBLT: with it the pointer flickers, and the composited
	// desktop already includes layered windows.
	if ok, _, err := procBitBlt.Call(uintptr(g.mem), 0, 0, uintptr(w), uintptr(h), uintptr(g.screen), 0, 0, srcCopy); ok == 0 {
		// The secure desktop (UAC, the lock screen) cannot be read.
		return nil, 0, 0, fmt.Errorf("could not read the screen: %v", err)
	}
	procGdiFlush.Call() //nolint:errcheck
	size := w * h * 4
	if cap(dst) < size {
		dst = make([]byte, size)
	}
	dst = dst[:size]
	copy(dst, unsafe.Slice((*byte)(g.bits), size))
	return dst, w, h, nil
}

func (g *gdi) Cursor() (int, int, uint32, error) {
	ci := cursorInfo{}
	ci.Size = uint32(unsafe.Sizeof(ci))
	if ok, _, err := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci))); ok == 0 {
		return 0, 0, 0, err
	}
	handle := ci.Cursor
	if ci.Flags&cursorShowing == 0 {
		handle = 0
	}
	g.curMu.Lock()
	g.cursor = handle
	g.curMu.Unlock()
	// A cursor's handle names its shape: the standard ones are shared.
	return int(ci.X), int(ci.Y), uint32(handle) ^ uint32(handle>>32), nil
}

func (g *gdi) CursorShape() (CursorShape, error) {
	g.curMu.Lock()
	defer g.curMu.Unlock()
	serial := uint32(g.cursor) ^ uint32(g.cursor>>32)
	if g.shape.W > 0 && g.shape.Serial == serial {
		return g.shape, nil
	}
	if g.cursor == 0 {
		// Hidden: a single clear pixel.
		g.shape = CursorShape{W: 1, H: 1, RGBA: make([]byte, 4), Serial: serial}
		return g.shape, nil
	}
	shape, err := drawCursor(g.cursor)
	if err != nil {
		return CursorShape{}, err
	}
	shape.Serial = serial
	g.shape = shape
	return shape, nil
}

// drawCursor draws a cursor on black and on white: what changes between
// the two is its transparency. That covers color cursors, masked ones and
// the old monochrome kind alike.
func drawCursor(cursor uintptr) (CursorShape, error) {
	var ii iconInfo
	if ok, _, err := procGetIconInfo.Call(cursor, uintptr(unsafe.Pointer(&ii))); ok == 0 {
		return CursorShape{}, err
	}
	var bm bitmap
	w, h := 0, 0
	if ii.Color != 0 {
		procGetObjectW.Call(uintptr(ii.Color), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))) //nolint:errcheck
		w, h = int(bm.Width), int(bm.Height)
	} else if ii.Mask != 0 {
		// A monochrome cursor's mask holds AND above XOR.
		procGetObjectW.Call(uintptr(ii.Mask), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))) //nolint:errcheck
		w, h = int(bm.Width), int(bm.Height)/2
	}
	for _, b := range []windows.Handle{ii.Color, ii.Mask} {
		if b != 0 {
			procDeleteObject.Call(uintptr(b)) //nolint:errcheck
		}
	}
	if w <= 0 || h <= 0 || w > 512 || h > 512 {
		return CursorShape{}, errors.New("no cursor image")
	}
	black, err := drawOn(cursor, w, h, 0x00)
	if err != nil {
		return CursorShape{}, err
	}
	white, err := drawOn(cursor, w, h, 0xff)
	if err != nil {
		return CursorShape{}, err
	}
	return CursorShape{W: w, H: h, HotX: int(ii.HotX), HotY: int(ii.HotY), RGBA: alphaFromBackgrounds(black, white)}, nil
}

// alphaFromBackgrounds recovers RGBA from the same image drawn over black
// and over white (both BGRX): a pixel that lets the background through by
// a shows it as white minus black.
func alphaFromBackgrounds(black, white []byte) []byte {
	rgba := make([]byte, len(black))
	for i := 0; i+3 < len(black); i += 4 {
		// The green channel decides; an inverting pixel comes out opaque.
		a := 255 - (int(white[i+1]) - int(black[i+1]))
		a = max(0, min(255, a))
		rgba[i+3] = byte(a)
		if a == 0 {
			continue
		}
		for c := 0; c < 3; c++ { // BGR to RGB, out of premultiplied
			rgba[i+2-c] = byte(min(255, int(black[i+c])*255/a))
		}
	}
	return rgba
}

func drawOn(cursor uintptr, w, h int, bg byte) ([]byte, error) {
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(dc) //nolint:errcheck
	hdr := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	var bits unsafe.Pointer
	dib, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&hdr)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		return nil, errors.New("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(dib) //nolint:errcheck
	old, _, _ := procSelectObject.Call(dc, dib)
	defer procSelectObject.Call(dc, old) //nolint:errcheck
	px := unsafe.Slice((*byte)(bits), w*h*4)
	for i := range px {
		px[i] = bg
	}
	if ok, _, err := procDrawIconEx.Call(dc, 0, 0, cursor, uintptr(w), uintptr(h), 0, 0, diNormal); ok == 0 {
		return nil, err
	}
	procGdiFlush.Call() //nolint:errcheck
	return append([]byte(nil), px...), nil
}

func sendInputs(inputs unsafe.Pointer, n int, size uintptr) error {
	sent, _, err := procSendInput.Call(uintptr(n), uintptr(inputs), size)
	if int(sent) != n {
		// UIPI: an elevated window in front takes nothing from a
		// process that is not.
		return fmt.Errorf("Windows refused the input: %v", err)
	}
	return nil
}

func sendMouse(in ...mouseInput) error {
	for i := range in {
		in[i].Type = inputMouse
	}
	return sendInputs(unsafe.Pointer(&in[0]), len(in), unsafe.Sizeof(in[0]))
}

func sendKeys(in ...keyInput) error {
	for i := range in {
		in[i].Type = inputKeyboard
	}
	return sendInputs(unsafe.Pointer(&in[0]), len(in), unsafe.Sizeof(in[0]))
}

// normalize maps a pixel to SendInput's 0–65535 range so that it lands
// on that pixel exactly.
func normalize(p, size int) int32 {
	if size <= 1 {
		return 0
	}
	return int32((p*65536 + size - 1) / size)
}

func (g *gdi) Move(x, y int) error {
	w, h := g.Size()
	return sendMouse(mouseInput{DX: normalize(x, w), DY: normalize(y, h), Flags: mouseMove | mouseAbsolute})
}

// Button presses a button where the pointer is; Windows counts double
// clicks itself.
func (g *gdi) Button(b int, down bool) error {
	var in mouseInput
	switch b {
	case 1, 2, 3:
		// Left 0x2/0x4, middle 0x20/0x40, right 0x8/0x10.
		flags := map[int]uint32{1: 0x0002, 2: 0x0020, 3: 0x0008}[b]
		if !down {
			flags <<= 1
		}
		in.Flags = flags
	case 4, 5, 6, 7:
		if !down {
			return nil
		}
		// Positive is up on the wheel and right on the tilt.
		delta := map[int]int32{4: wheelDelta, 5: -wheelDelta, 6: -wheelDelta, 7: wheelDelta}[b]
		in.MouseData = uint32(delta)
		in.Flags = mouseWheel
		if b >= 6 {
			in.Flags = mouseHWheel
		}
	default:
		return fmt.Errorf("no mouse button %d", b)
	}
	return sendMouse(in)
}

// vkInput presses or releases a virtual key, with its scan code for
// programs that read those. Extended keys are the right Ctrl, the arrows
// and their kind.
func vkInput(vk uint16, extended, down bool) keyInput {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
	in := keyInput{VK: vk, Scan: uint16(scan)}
	if extended {
		in.Flags |= keyExtended
	}
	if !down {
		in.Flags |= keyUp
	}
	return in
}

// charKey is the key that types r in the current layout and the modifiers
// it needs: 1 Shift, 2 Ctrl, 4 Alt.
func charKey(r rune) (vk uint16, mods byte, ok bool) {
	if r > 0xffff {
		return 0, 0, false
	}
	res, _, _ := procVkKeyScanW.Call(uintptr(r))
	if int16(res) == -1 {
		return 0, 0, false
	}
	return uint16(res & 0xff), byte(res >> 8), true
}

func unicodeInputs(r rune, down bool) []keyInput {
	var in []keyInput
	for _, u := range utf16.Encode([]rune{r}) {
		k := keyInput{Scan: u, Flags: keyUnicode}
		if !down {
			k.Flags |= keyUp
		}
		in = append(in, k)
	}
	return in
}

// Key presses the key that types keysym, as is: held modifiers apply. A
// character the layout lacks goes as that character.
func (g *gdi) Key(keysym uint32, down bool) error {
	g.inMu.Lock()
	defer g.inMu.Unlock()
	if k, ok := winKeys[keysym]; ok {
		if keysym == 0xffe1 || keysym == 0xffe2 {
			if down {
				g.shifted[keysym] = true
			} else {
				delete(g.shifted, keysym)
			}
		}
		return sendKeys(vkInput(k.vk, k.extended, down))
	}
	r, ok := RuneForKeysym(keysym)
	if !ok {
		return fmt.Errorf("no key for keysym %#x", keysym)
	}
	if vk, _, ok := charKey(r); ok {
		return sendKeys(vkInput(vk, false, down))
	}
	return sendKeys(unicodeInputs(r, down)...)
}

// Type taps the key for r, adding or lifting Shift as the layout needs and
// allowing for Caps Lock. A character that needs AltGr, or is on no key,
// goes as that character.
func (g *gdi) Type(r rune) error {
	g.inMu.Lock()
	defer g.inMu.Unlock()
	if ks := KeysymForRune(r); ks >= 0xff00 && ks <= 0xffff {
		if k, ok := winKeys[ks]; ok { // Return, Tab, BackSpace
			return sendKeys(vkInput(k.vk, k.extended, true), vkInput(k.vk, k.extended, false))
		}
	}
	vk, mods, ok := charKey(r)
	if !ok || mods&^1 != 0 {
		return sendKeys(append(unicodeInputs(r, true), unicodeInputs(r, false)...)...)
	}
	shift := mods&1 != 0
	if vk >= 'A' && vk <= 'Z' {
		if caps, _, _ := procGetKeyState.Call(vkCapital); caps&1 != 0 {
			shift = !shift
		}
	}
	var in []keyInput
	held := len(g.shifted) > 0
	switch {
	case shift && !held:
		in = append(in, vkInput(vkShift, false, true), vkInput(vk, false, true), vkInput(vk, false, false), vkInput(vkShift, false, false))
	case !shift && held:
		for ks := range g.shifted {
			in = append(in, vkInput(winKeys[ks].vk, false, false))
		}
		in = append(in, vkInput(vk, false, true), vkInput(vk, false, false))
		for ks := range g.shifted {
			in = append(in, vkInput(winKeys[ks].vk, false, true))
		}
	default:
		in = append(in, vkInput(vk, false, true), vkInput(vk, false, false))
	}
	return sendKeys(in...)
}

func clipboardSequence() int64 {
	n, _, _ := procClipboardSequence.Call()
	return int64(n)
}

// openClipboard opens the clipboard on this thread, waiting a little for
// another program that holds it.
func openClipboard() error {
	var err error
	for range maxClipboardTries {
		var ok uintptr
		if ok, _, err = procOpenClipboard.Call(0); ok != 0 {
			return nil
		}
		time.Sleep(clipboardRetryWait)
	}
	return fmt.Errorf("the clipboard is busy: %v", err)
}

func (g *gdi) Clipboard() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call() //nolint:errcheck
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil // it has no text
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return "", errors.New("could not read the clipboard")
	}
	defer procGlobalUnlock.Call(h) //nolint:errcheck
	return windows.UTF16PtrToString((*uint16)(pointer(p))), nil
}

func (g *gdi) SetClipboard(text string) error {
	units, err := windows.UTF16FromString(text)
	if err != nil {
		return errors.New("clipboard text cannot hold a NUL")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call() //nolint:errcheck
	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		return err
	}
	size := uintptr(len(units) * 2)
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h) //nolint:errcheck
		return err
	}
	copy(unsafe.Slice((*uint16)(pointer(p)), len(units)), units)
	procGlobalUnlock.Call(h) //nolint:errcheck
	// The system owns the memory once SetClipboardData takes it.
	if ok, _, err := procSetClipboardData.Call(cfUnicodeText, h); ok == 0 {
		procGlobalFree.Call(h) //nolint:errcheck
		return err
	}
	g.clip.setOwn(clipboardSequence())
	return nil
}

// pointer is memory the system handed out as a number, as GlobalLock does.
func pointer(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

func (g *gdi) ClipboardChanges() (<-chan string, func()) { return g.clip.subscribe() }

func (g *gdi) Close() {
	g.clip.close()
	g.inMu.Lock()
	for ks := range g.shifted {
		sendKeys(vkInput(winKeys[ks].vk, false, false)) //nolint:errcheck
		delete(g.shifted, ks)
	}
	g.inMu.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	g.freeLocked()
}

type winKey struct {
	vk       uint16
	extended bool
}

// winKeys are the keys that type no character, by keysym. Super is the
// Windows key.
var winKeys = map[uint32]winKey{
	0xffe1: {0xa0, false}, 0xffe2: {0xa1, false}, // Shift
	0xffe3: {0xa2, false}, 0xffe4: {0xa3, true}, // Ctrl
	0xffe9: {0xa4, false}, 0xffea: {0xa5, true}, // Alt
	0xffeb: {0x5b, true}, 0xffec: {0x5c, true}, // Super
	0xffe7: {0x5b, true}, 0xffe8: {0x5c, true}, // Meta
	0xff0d: {0x0d, false}, 0xff8d: {0x0d, true}, 0xff09: {0x09, false},
	0xff08: {0x08, false}, 0xff1b: {0x1b, false}, 0xffff: {0x2e, true},
	0xff50: {0x24, true}, 0xff57: {0x23, true}, 0xff55: {0x21, true}, 0xff56: {0x22, true},
	0xff51: {0x25, true}, 0xff52: {0x26, true}, 0xff53: {0x27, true}, 0xff54: {0x28, true},
	0xff63: {0x2d, true}, 0xff67: {0x5d, true}, 0xff13: {0x13, false}, 0xff14: {0x91, false},
	0xff61: {0x2c, true}, 0xff7f: {0x90, true}, 0xffe5: {0x14, false}, 0x20: {0x20, false},
	0x1008ff12: {0xad, true}, 0x1008ff11: {0xae, true}, 0x1008ff13: {0xaf, true},
	// F1–F24
	0xffbe: {0x70, false}, 0xffbf: {0x71, false}, 0xffc0: {0x72, false}, 0xffc1: {0x73, false},
	0xffc2: {0x74, false}, 0xffc3: {0x75, false}, 0xffc4: {0x76, false}, 0xffc5: {0x77, false},
	0xffc6: {0x78, false}, 0xffc7: {0x79, false}, 0xffc8: {0x7a, false}, 0xffc9: {0x7b, false},
	0xffca: {0x7c, false}, 0xffcb: {0x7d, false}, 0xffcc: {0x7e, false}, 0xffcd: {0x7f, false},
	0xffce: {0x80, false}, 0xffcf: {0x81, false}, 0xffd0: {0x82, false}, 0xffd1: {0x83, false},
	0xffd2: {0x84, false}, 0xffd3: {0x85, false}, 0xffd4: {0x86, false}, 0xffd5: {0x87, false},
}
