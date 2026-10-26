package screen

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// quartz is the macOS main display: frames from a CGDisplayStream, which
// wakes only when the screen changes, input posted as CGEvents, the
// pointer's image from NSCursor and text from NSPasteboard. Everything is
// called through purego, so it builds without cgo.
//
// The screen is in pixels; macOS places the pointer in points, which a
// Retina screen has half as many of on each side.
type quartz struct {
	mu      sync.Mutex
	closed  bool
	display uint32
	stream  uintptr
	surface uintptr // the latest frame, retained; 0 until the first one
	w, h    int     // the stream's size in pixels
	scale   float64 // pixels per point
	first   chan struct{}

	changes changeSignal

	curMu sync.Mutex
	shape CursorShape

	inMu    sync.Mutex
	mods    map[uint32]bool // modifier keysyms held
	buttons map[int]bool
	click   struct {
		at     time.Time
		x, y   int
		button int
		count  int64
	}
	keymap   map[rune]macKey
	keymapAt time.Time
	trusted  bool
	askedAt  time.Time

	clip clipWatch
}

type macKey struct {
	code  uint16
	shift bool
}

var (
	cg struct {
		mainDisplayID       func() uint32
		pointsWide          func(uint32) uintptr
		pointsHigh          func(uint32) uintptr
		copyMode            func(uint32) uintptr
		modePixelsWide      func(uintptr) uintptr
		modePixelsHigh      func(uintptr) uintptr
		modeRelease         func(uintptr)
		preflightCapture    func() bool
		requestCapture      func() bool
		streamCreate        func(uint32, uintptr, uintptr, int32, uintptr, uintptr, objc.Block) uintptr
		streamStart         func(uintptr) int32
		streamStop          func(uintptr) int32
		eventCreate         func(uintptr) uintptr
		eventLocation       func(uintptr) cgPoint
		mouseEvent          func(uintptr, uint32, cgPoint, uint32) uintptr
		keyEvent            func(uintptr, uint16, bool) uintptr
		scrollEvent         func(uintptr, uint32, uint32, int32, int32, int32) uintptr
		setFlags            func(uintptr, uint64)
		setType             func(uintptr, uint32)
		setField            func(uintptr, uint32, int64)
		setUnicode          func(uintptr, uintptr, *uint16)
		post                func(uint32, uintptr)
		colorSpace          func() uintptr
		colorSpaceRelease   func(uintptr)
		bitmapCreate        func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uint32) uintptr
		bitmapData          func(uintptr) unsafe.Pointer
		contextRelease      func(uintptr)
		drawImage           func(uintptr, cgRect, uintptr)
		showCursorKey       uintptr
		minFrameTimeKey     uintptr
		cfRetain            func(uintptr) uintptr
		cfRelease           func(uintptr)
		cfDataPtr           func(uintptr) uintptr
		surfaceLock         func(uintptr, uint32, uintptr) int32
		surfaceUnlock       func(uintptr, uint32, uintptr) int32
		surfaceBase         func(uintptr) unsafe.Pointer
		surfaceW            func(uintptr) uintptr
		surfaceH            func(uintptr) uintptr
		surfaceStride       func(uintptr) uintptr
		surfaceUse          func(uintptr)
		surfaceUnuse        func(uintptr)
		queueCreate         func(*byte, uintptr) uintptr
		axTrusted           func() bool
		axTrustedPrompt     func(uintptr) bool
		axPromptKey         uintptr
		tisLayout           func() uintptr
		tisProperty         func(uintptr, uintptr) uintptr
		tisLayoutDataKey    uintptr
		kbdType             func() uint8
		keyTranslate        func(uintptr, uint16, uint16, uint32, uint32, uint32, *uint32, uintptr, *uintptr, *uint16) int32
		poolPush            func() uintptr
		poolPop             func(uintptr)
		pasteboardStringKey objc.ID
		dispatchSync        func(uintptr, objc.Block)
		mainQueue           uintptr
		nsApp               *objc.ID
	}
	cgOnce  sync.Once
	cgErr   error
	cgBlock objc.Block // one frame handler for every stream; purego callbacks are never freed
	cgQueue uintptr
	// layout runs the keyboard layout reads; see onLayoutThread.
	layout struct {
		sync.Mutex
		f     func()
		block objc.Block
		work  chan func()
	}
	// cgCurrent is the open display, which cgBlock hands frames to.
	cgCurrent struct {
		sync.Mutex
		q *quartz
	}
)

// Selectors, registered once: registering takes the runtime's global lock.
var (
	selCGImageForProposedRectContextHints = objc.RegisterName("CGImageForProposedRect:context:hints:")
	selUTF8String                         = objc.RegisterName("UTF8String")
	selAlloc                              = objc.RegisterName("alloc")
	selArrowCursor                        = objc.RegisterName("arrowCursor")
	selAutorelease                        = objc.RegisterName("autorelease")
	selChangeCount                        = objc.RegisterName("changeCount")
	selClearContents                      = objc.RegisterName("clearContents")
	selCurrentSystemCursor                = objc.RegisterName("currentSystemCursor")
	selDictionaryWithObjectsForKeysCount  = objc.RegisterName("dictionaryWithObjects:forKeys:count:")
	selGeneralPasteboard                  = objc.RegisterName("generalPasteboard")
	selHotSpot                            = objc.RegisterName("hotSpot")
	selImage                              = objc.RegisterName("image")
	selInitWithBytesLengthEncoding        = objc.RegisterName("initWithBytes:length:encoding:")
	selIsRunning                          = objc.RegisterName("isRunning")
	selLengthOfBytesUsingEncoding         = objc.RegisterName("lengthOfBytesUsingEncoding:")
	selNumberWithBool                     = objc.RegisterName("numberWithBool:")
	selNumberWithDouble                   = objc.RegisterName("numberWithDouble:")
	selSetStringForType                   = objc.RegisterName("setString:forType:")
	selSize                               = objc.RegisterName("size")
	selStringForType                      = objc.RegisterName("stringForType:")
)

type cgPoint struct{ X, Y float64 }
type cgRect struct{ X, Y, W, H float64 }
type nsSize struct{ W, H float64 }

const (
	streamFrameComplete = 0
	lockReadOnly        = 1
	hidEventTap         = 0
	flagsChanged        = 12
	clickStateField     = 1
	scrollUnitLine      = 1
	pixelFormatBGRA     = 'B'<<24 | 'G'<<16 | 'R'<<8 | 'A'
	doubleClickTime     = 500 * time.Millisecond
	flagShift           = 0x20000
	flagControl         = 0x40000
	flagOption          = 0x80000
	flagCommand         = 0x100000
)

func loadQuartz() (err error) {
	defer func() {
		// RegisterLibFunc panics on a missing symbol: an older macOS.
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	open := func(path string) uintptr {
		h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		return h
	}
	fn := func(lib uintptr, ptr any, name string) { purego.RegisterLibFunc(ptr, lib, name) }
	symbol := func(lib uintptr, name string) uintptr {
		sym, err := purego.Dlsym(lib, name)
		if err != nil {
			panic(err)
		}
		return sym
	}
	global := func(lib uintptr, name string) uintptr {
		sym := symbol(lib, name)
		return **(**uintptr)(unsafe.Pointer(&sym))
	}
	g := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	ios := open("/System/Library/Frameworks/IOSurface.framework/IOSurface")
	sys := open("/usr/lib/libSystem.B.dylib")
	ax := open("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
	carbon := open("/System/Library/Frameworks/Carbon.framework/Carbon")
	appkit := open("/System/Library/Frameworks/AppKit.framework/AppKit")
	lobjc := open("/usr/lib/libobjc.A.dylib")

	fn(g, &cg.mainDisplayID, "CGMainDisplayID")
	fn(g, &cg.pointsWide, "CGDisplayPixelsWide")
	fn(g, &cg.pointsHigh, "CGDisplayPixelsHigh")
	fn(g, &cg.copyMode, "CGDisplayCopyDisplayMode")
	fn(g, &cg.modePixelsWide, "CGDisplayModeGetPixelWidth")
	fn(g, &cg.modePixelsHigh, "CGDisplayModeGetPixelHeight")
	fn(g, &cg.modeRelease, "CGDisplayModeRelease")
	fn(g, &cg.preflightCapture, "CGPreflightScreenCaptureAccess")
	fn(g, &cg.requestCapture, "CGRequestScreenCaptureAccess")
	fn(g, &cg.streamCreate, "CGDisplayStreamCreateWithDispatchQueue")
	fn(g, &cg.streamStart, "CGDisplayStreamStart")
	fn(g, &cg.streamStop, "CGDisplayStreamStop")
	fn(g, &cg.eventCreate, "CGEventCreate")
	fn(g, &cg.eventLocation, "CGEventGetLocation")
	fn(g, &cg.mouseEvent, "CGEventCreateMouseEvent")
	fn(g, &cg.keyEvent, "CGEventCreateKeyboardEvent")
	// The variadic CGEventCreateScrollWheelEvent cannot be called portably.
	fn(g, &cg.scrollEvent, "CGEventCreateScrollWheelEvent2")
	fn(g, &cg.setFlags, "CGEventSetFlags")
	fn(g, &cg.setType, "CGEventSetType")
	fn(g, &cg.setField, "CGEventSetIntegerValueField")
	fn(g, &cg.setUnicode, "CGEventKeyboardSetUnicodeString")
	fn(g, &cg.post, "CGEventPost")
	fn(g, &cg.colorSpace, "CGColorSpaceCreateDeviceRGB")
	fn(g, &cg.colorSpaceRelease, "CGColorSpaceRelease")
	fn(g, &cg.bitmapCreate, "CGBitmapContextCreate")
	fn(g, &cg.bitmapData, "CGBitmapContextGetData")
	fn(g, &cg.contextRelease, "CGContextRelease")
	fn(g, &cg.drawImage, "CGContextDrawImage")
	cg.showCursorKey = global(g, "kCGDisplayStreamShowCursor")
	cg.minFrameTimeKey = global(g, "kCGDisplayStreamMinimumFrameTime")
	fn(cf, &cg.cfRetain, "CFRetain")
	fn(cf, &cg.cfRelease, "CFRelease")
	fn(cf, &cg.cfDataPtr, "CFDataGetBytePtr")
	fn(ios, &cg.surfaceLock, "IOSurfaceLock")
	fn(ios, &cg.surfaceUnlock, "IOSurfaceUnlock")
	fn(ios, &cg.surfaceBase, "IOSurfaceGetBaseAddress")
	fn(ios, &cg.surfaceW, "IOSurfaceGetWidth")
	fn(ios, &cg.surfaceH, "IOSurfaceGetHeight")
	fn(ios, &cg.surfaceStride, "IOSurfaceGetBytesPerRow")
	fn(ios, &cg.surfaceUse, "IOSurfaceIncrementUseCount")
	fn(ios, &cg.surfaceUnuse, "IOSurfaceDecrementUseCount")
	fn(sys, &cg.queueCreate, "dispatch_queue_create")
	fn(sys, &cg.dispatchSync, "dispatch_sync")
	cg.mainQueue = symbol(sys, "_dispatch_main_q")
	nsApp := symbol(appkit, "NSApp")
	cg.nsApp = *(**objc.ID)(unsafe.Pointer(&nsApp))
	fn(ax, &cg.axTrusted, "AXIsProcessTrusted")
	fn(ax, &cg.axTrustedPrompt, "AXIsProcessTrustedWithOptions")
	cg.axPromptKey = global(ax, "kAXTrustedCheckOptionPrompt")
	fn(carbon, &cg.tisLayout, "TISCopyCurrentKeyboardLayoutInputSource")
	fn(carbon, &cg.tisProperty, "TISGetInputSourceProperty")
	cg.tisLayoutDataKey = global(carbon, "kTISPropertyUnicodeKeyLayoutData")
	fn(carbon, &cg.kbdType, "LMGetKbdType")
	fn(carbon, &cg.keyTranslate, "UCKeyTranslate")
	fn(lobjc, &cg.poolPush, "objc_autoreleasePoolPush")
	fn(lobjc, &cg.poolPop, "objc_autoreleasePoolPop")
	cg.pasteboardStringKey = objc.ID(global(appkit, "NSPasteboardTypeString"))

	name := []byte("agenttik.screen\x00")
	cgQueue = cg.queueCreate(&name[0], 0)
	cgBlock = objc.NewBlock(func(_ objc.Block, status int32, _ uint64, surface, _ uintptr) {
		cgCurrent.Lock()
		q := cgCurrent.q
		cgCurrent.Unlock()
		if q != nil {
			q.frame(status, surface)
		}
	})
	layout.block = objc.NewBlock(func(objc.Block) { layout.f() })
	layout.work = make(chan func())
	go func() {
		runtime.LockOSThread()
		for f := range layout.work {
			f()
		}
	}()
	return nil
}

// Open starts watching the main display. macOS asks the person at the
// machine, once, to let agenttik record the screen and, separately, to
// control it.
func Open() (Display, error) {
	cgOnce.Do(func() { cgErr = loadQuartz() })
	if cgErr != nil {
		return nil, Unavailable("screen capture needs macOS 13 or later: " + cgErr.Error())
	}
	if !cg.preflightCapture() {
		cg.requestCapture()
		return nil, Unavailable("macOS has not allowed agenttik to record the screen. Allow it in System Settings › Privacy & Security › Screen & System Audio Recording, then restart agenttik.")
	}
	q := &quartz{mods: map[uint32]bool{}, buttons: map[int]bool{}}
	q.clip = clipWatch{seq: q.changeCount, read: q.Clipboard}
	cgCurrent.Lock()
	if cgCurrent.q != nil {
		cgCurrent.Unlock()
		return nil, errors.New("the screen is already open")
	}
	cgCurrent.q = q
	cgCurrent.Unlock()
	q.mu.Lock()
	err := q.startLocked()
	first := q.first
	q.mu.Unlock()
	if err == nil {
		select {
		case <-first:
		case <-time.After(3 * time.Second):
			err = Unavailable("the screen sent no picture")
		}
	}
	if err != nil {
		q.Close()
		return nil, err
	}
	return q, nil
}

// startLocked (re)starts the stream at the main display's size in pixels.
// The last frame stays until the new stream's first replaces it.
func (q *quartz) startLocked() error {
	q.stopLocked()
	q.display = cg.mainDisplayID()
	w, h := modePixels(q.display)
	pw := int(cg.pointsWide(q.display))
	if w == 0 || h == 0 || pw == 0 {
		return Unavailable("no display")
	}
	q.scale = float64(w) / float64(pw)
	q.first = make(chan struct{})
	var stream uintptr
	withPool(func() {
		props := nsDictionary(
			[]objc.ID{objc.ID(cg.showCursorKey), objc.ID(cg.minFrameTimeKey)},
			[]objc.ID{nsBool(false), nsDouble(frameInterval.Seconds())},
		)
		stream = cg.streamCreate(q.display, uintptr(w), uintptr(h), pixelFormatBGRA, uintptr(props), cgQueue, cgBlock)
	})
	if stream == 0 {
		return Unavailable("macOS would not stream the screen")
	}
	if cg.streamStart(stream) != 0 {
		cg.cfRelease(stream)
		return Unavailable("macOS would not start streaming the screen")
	}
	q.stream, q.w, q.h = stream, w, h
	return nil
}

func (q *quartz) stopLocked() {
	if q.stream != 0 {
		cg.streamStop(q.stream)
		cg.cfRelease(q.stream)
		q.stream = 0
	}
}

func modePixels(display uint32) (int, int) {
	mode := cg.copyMode(display)
	if mode == 0 {
		return 0, 0
	}
	defer cg.modeRelease(mode)
	return int(cg.modePixelsWide(mode)), int(cg.modePixelsHigh(mode))
}

// frame keeps the stream's newest picture. It runs on the stream's queue.
func (q *quartz) frame(status int32, surface uintptr) {
	// Idle, blank and stopped frames carry no picture. A stream stops when
	// it is replaced or its display goes, which Grab notices on its own.
	if status != streamFrameComplete || surface == 0 {
		return
	}
	cg.cfRetain(surface)
	cg.surfaceUse(surface)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		releaseSurface(surface)
		return
	}
	old := q.surface
	q.surface = surface
	if q.first != nil {
		close(q.first)
		q.first = nil
	}
	q.mu.Unlock()
	if old != 0 {
		releaseSurface(old)
	}
	q.changes.bump()
}

func releaseSurface(s uintptr) {
	cg.surfaceUnuse(s)
	cg.cfRelease(s)
}

func (q *quartz) Size() (int, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.w, q.h
}

func (q *quartz) Changes() uint64 { return q.changes.count.Load() }

func (q *quartz) ChangedSince(n uint64) <-chan struct{} { return q.changes.since(n) }

func (q *quartz) Grab(dst []byte) ([]byte, int, int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil, 0, 0, errors.New("display closed")
	}
	// A new resolution, or another main display, needs a new stream.
	if id := cg.mainDisplayID(); q.stream == 0 || id != q.display {
		q.startLocked() //nolint:errcheck // the old frame stands in until it works
	} else if w, h := modePixels(id); w != q.w || h != q.h {
		q.startLocked() //nolint:errcheck
	}
	s := q.surface
	if s == 0 {
		return nil, 0, 0, errors.New("no frame yet")
	}
	if cg.surfaceLock(s, lockReadOnly, 0) != 0 {
		return nil, 0, 0, errors.New("could not read the screen")
	}
	defer cg.surfaceUnlock(s, lockReadOnly, 0)
	w, h, stride := int(cg.surfaceW(s)), int(cg.surfaceH(s)), int(cg.surfaceStride(s))
	base := cg.surfaceBase(s)
	if base == nil || stride < w*4 {
		return nil, 0, 0, errors.New("unexpected screen pixel layout")
	}
	size := w * h * 4
	if cap(dst) < size {
		dst = make([]byte, size)
	}
	dst = dst[:size]
	src := unsafe.Slice((*byte)(base), stride*h)
	if stride == w*4 {
		copy(dst, src)
	} else {
		for y := 0; y < h; y++ {
			copy(dst[y*w*4:(y+1)*w*4], src[y*stride:])
		}
	}
	return dst, w, h, nil
}

// pixels turns a point into screen pixels, and points back.
func (q *quartz) pixels(p cgPoint) (int, int) {
	q.mu.Lock()
	s := q.scale
	q.mu.Unlock()
	return int(math.Round(p.X * s)), int(math.Round(p.Y * s))
}

func (q *quartz) points(x, y int) cgPoint {
	q.mu.Lock()
	s := q.scale
	q.mu.Unlock()
	return cgPoint{float64(x) / s, float64(y) / s}
}

func (q *quartz) location() cgPoint {
	ev := cg.eventCreate(0)
	if ev == 0 {
		return cgPoint{}
	}
	defer cg.cfRelease(ev)
	return cg.eventLocation(ev)
}

// Cursor reads the pointer's position and its image. macOS has no serial
// for the image, so it is a hash of the pixels; a cursor is a few kilobytes.
func (q *quartz) Cursor() (int, int, uint32, error) {
	x, y := q.pixels(q.location())
	shape, err := q.readShape()
	if err != nil {
		return x, y, 0, nil
	}
	q.curMu.Lock()
	q.shape = shape
	q.curMu.Unlock()
	return x, y, shape.Serial, nil
}

func (q *quartz) CursorShape() (CursorShape, error) {
	q.curMu.Lock()
	shape := q.shape
	q.curMu.Unlock()
	if shape.W > 0 {
		return shape, nil
	}
	return q.readShape()
}

func (q *quartz) readShape() (shape CursorShape, err error) {
	q.mu.Lock()
	scale := q.scale
	q.mu.Unlock()
	withPool(func() {
		nsCursor := objc.ID(objc.GetClass("NSCursor"))
		cur := nsCursor.Send(selCurrentSystemCursor)
		if cur == 0 {
			cur = nsCursor.Send(selArrowCursor)
		}
		img := cur.Send(selImage)
		if img == 0 {
			err = errors.New("no cursor image")
			return
		}
		hot := objc.Send[cgPoint](cur, selHotSpot)
		size := objc.Send[nsSize](img, selSize)
		cgImage := img.Send(selCGImageForProposedRectContextHints, uintptr(0), uintptr(0), uintptr(0))
		w, h := int(math.Round(size.W*scale)), int(math.Round(size.H*scale))
		if cgImage == 0 || w <= 0 || h <= 0 || w > 512 || h > 512 {
			err = errors.New("no cursor image")
			return
		}
		rgba, ok := drawRGBA(uintptr(cgImage), w, h)
		if !ok {
			err = errors.New("could not draw the cursor")
			return
		}
		shape = CursorShape{W: w, H: h, HotX: int(math.Round(hot.X * scale)), HotY: int(math.Round(hot.Y * scale)), RGBA: rgba}
	})
	if err != nil {
		return CursorShape{}, err
	}
	hash := fnv.New32a()
	hash.Write(shape.RGBA)
	hash.Write([]byte{byte(shape.HotX), byte(shape.HotY), byte(shape.W), byte(shape.H)})
	shape.Serial = hash.Sum32()
	return shape, nil
}

// drawRGBA draws a CGImage at w×h and answers its straight RGBA pixels,
// top row first.
func drawRGBA(img uintptr, w, h int) ([]byte, bool) {
	cs := cg.colorSpace()
	defer cg.colorSpaceRelease(cs)
	// Premultiplied alpha last, big-endian 32-bit: R, G, B, A in memory.
	ctx := cg.bitmapCreate(0, uintptr(w), uintptr(h), 8, uintptr(w*4), cs, 1|4<<12)
	if ctx == 0 {
		return nil, false
	}
	defer cg.contextRelease(ctx)
	cg.drawImage(ctx, cgRect{0, 0, float64(w), float64(h)}, img)
	data := cg.bitmapData(ctx)
	if data == nil {
		return nil, false
	}
	rgba := make([]byte, w*h*4)
	copy(rgba, unsafe.Slice((*byte)(data), len(rgba)))
	unpremultiply(rgba)
	return rgba, true
}

func unpremultiply(rgba []byte) {
	for i := 0; i+3 < len(rgba); i += 4 {
		a := int(rgba[i+3])
		if a != 0 && a != 255 {
			rgba[i] = byte(min(255, int(rgba[i])*255/a))
			rgba[i+1] = byte(min(255, int(rgba[i+1])*255/a))
			rgba[i+2] = byte(min(255, int(rgba[i+2])*255/a))
		}
	}
}

// ControlReason says why input will not arrive: macOS drops posted events
// until the person at the machine allows agenttik in Accessibility.
func (q *quartz) ControlReason() string {
	if q.canControl() {
		return ""
	}
	return "macOS has not allowed agenttik to control this computer, so the mouse and keyboard do nothing. Allow it in System Settings › Privacy & Security › Accessibility."
}

func (q *quartz) canControl() bool {
	q.inMu.Lock()
	defer q.inMu.Unlock()
	return q.canControlLocked()
}

// canControlLocked asks macOS, at most every two seconds until it says yes.
// The first no also shows macOS's own prompt.
func (q *quartz) canControlLocked() bool {
	if q.trusted || time.Since(q.askedAt) < 2*time.Second {
		return q.trusted
	}
	q.askedAt = time.Now()
	q.trusted = cg.axTrusted()
	if !q.trusted {
		axPromptOnce.Do(func() {
			withPool(func() {
				opts := nsDictionary([]objc.ID{objc.ID(cg.axPromptKey)}, []objc.ID{nsBool(true)})
				cg.axTrustedPrompt(uintptr(opts))
			})
		})
	}
	return q.trusted
}

var axPromptOnce sync.Once

var errNoControl = errors.New("macOS has not allowed agenttik to control this computer (System Settings › Privacy & Security › Accessibility)")

// postLocked sends an event with the held modifiers and lets go of it.
func (q *quartz) postLocked(ev uintptr, flags uint64) error {
	if ev == 0 {
		return errors.New("could not create the input event")
	}
	defer cg.cfRelease(ev)
	if !q.canControlLocked() {
		return errNoControl
	}
	cg.setFlags(ev, flags)
	cg.post(hidEventTap, ev)
	return nil
}

func (q *quartz) flagsLocked() uint64 {
	var f uint64
	for ks := range q.mods {
		f |= macModifiers[ks].flag
	}
	return f
}

func (q *quartz) Move(x, y int) error {
	q.inMu.Lock()
	defer q.inMu.Unlock()
	kind, button := uint32(5), uint32(0) // moved
	switch {
	case q.buttons[1]:
		kind, button = 6, 0 // left dragged
	case q.buttons[3]:
		kind, button = 7, 1 // right dragged
	case q.buttons[2]:
		kind, button = 27, 2 // other dragged
	}
	return q.postLocked(cg.mouseEvent(0, kind, q.points(x, y), button), q.flagsLocked())
}

// Button presses a mouse button where the pointer is. Presses close in time
// and place count up, which is how macOS sees a double click.
func (q *quartz) Button(b int, down bool) error {
	q.inMu.Lock()
	defer q.inMu.Unlock()
	if b >= 4 && b <= 7 {
		if !down {
			return nil
		}
		// Positive is up and left, as on X's buttons 4 and 6.
		var dy, dx int32
		switch b {
		case 4:
			dy = 1
		case 5:
			dy = -1
		case 6:
			dx = 1
		case 7:
			dx = -1
		}
		return q.postLocked(cg.scrollEvent(0, scrollUnitLine, 2, dy, dx, 0), q.flagsLocked())
	}
	var kind, button uint32
	switch b {
	case 1:
		kind, button = 1, 0
	case 3:
		kind, button = 3, 1
	case 2:
		kind, button = 25, 2
	default:
		return fmt.Errorf("no mouse button %d", b)
	}
	if !down {
		kind++ // each up follows its down
	}
	p := q.location()
	x, y := int(p.X), int(p.Y)
	c := &q.click
	if down {
		if c.button == b && time.Since(c.at) < doubleClickTime && abs(x-c.x) <= 4 && abs(y-c.y) <= 4 {
			c.count++
		} else {
			c.count = 1
		}
		c.at, c.x, c.y, c.button = time.Now(), x, y, b
		q.buttons[b] = true
	} else {
		delete(q.buttons, b)
	}
	ev := cg.mouseEvent(0, kind, p, button)
	if ev != 0 {
		cg.setField(ev, clickStateField, max(c.count, 1))
	}
	return q.postLocked(ev, q.flagsLocked())
}

// Key presses the key that types keysym in the current layout, with the
// held modifiers. A character the layout lacks goes as that text.
func (q *quartz) Key(keysym uint32, down bool) error {
	q.inMu.Lock()
	defer q.inMu.Unlock()
	if m, ok := macModifiers[keysym]; ok {
		if down {
			q.mods[keysym] = true
		} else {
			delete(q.mods, keysym)
		}
		ev := cg.keyEvent(0, m.code, down)
		if ev != 0 {
			cg.setType(ev, flagsChanged)
		}
		return q.postLocked(ev, q.flagsLocked())
	}
	if code, ok := macSpecialKeys[keysym]; ok {
		return q.postLocked(cg.keyEvent(0, code, down), q.flagsLocked())
	}
	r, ok := RuneForKeysym(keysym)
	if !ok {
		return fmt.Errorf("no key for keysym %#x", keysym)
	}
	k, ok := q.keyForLocked(r)
	if !ok {
		return q.typeTextLocked(0, r, down, q.flagsLocked())
	}
	return q.postLocked(cg.keyEvent(0, k.code, down), q.flagsLocked())
}

// Type taps the key for r, with Shift as the layout needs. The event also
// carries r itself, so a character on no key still arrives.
func (q *quartz) Type(r rune) error {
	q.inMu.Lock()
	defer q.inMu.Unlock()
	if ks := KeysymForRune(r); ks >= 0xff00 && ks <= 0xffff {
		if code, ok := macSpecialKeys[ks]; ok { // Return, Tab, BackSpace
			if err := q.postLocked(cg.keyEvent(0, code, true), q.flagsLocked()); err != nil {
				return err
			}
			return q.postLocked(cg.keyEvent(0, code, false), q.flagsLocked())
		}
	}
	k, _ := q.keyForLocked(r)
	flags := q.flagsLocked() &^ flagShift
	if k.shift {
		flags |= flagShift
	}
	if err := q.typeTextLocked(k.code, r, true, flags); err != nil {
		return err
	}
	return q.typeTextLocked(k.code, r, false, flags)
}

func (q *quartz) typeTextLocked(code uint16, r rune, down bool, flags uint64) error {
	ev := cg.keyEvent(0, code, down)
	if ev != 0 {
		units := utf16Of(r)
		cg.setUnicode(ev, uintptr(len(units)), &units[0])
	}
	return q.postLocked(ev, flags)
}

func utf16Of(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)}
	}
	r -= 0x10000
	return []uint16{uint16(0xd800 + r>>10), uint16(0xdc00 + r&0x3ff)}
}

// keyForLocked finds the key that types r in the current layout, which is
// read again when two seconds old so a layout switch is seen.
func (q *quartz) keyForLocked(r rune) (macKey, bool) {
	if q.keymap == nil || time.Since(q.keymapAt) > 2*time.Second {
		onLayoutThread(func() { q.keymap = readKeymap() })
		q.keymapAt = time.Now()
	}
	if k, ok := q.keymap[r]; ok {
		return k, true
	}
	return macKey{}, false
}

// onLayoutThread runs f where HIToolbox lets this process read keyboard
// layouts. In a program built against a recent SDK it traps when an
// application reads them off the main thread, and aborts when a process with
// no application reads them from a thread other than its first. So: the main
// thread while an application runs there (the desktop window), else a thread
// kept for it.
func onLayoutThread(f func()) {
	if app := *cg.nsApp; app != 0 && objc.Send[bool](app, selIsRunning) {
		layout.Lock()
		defer layout.Unlock()
		layout.f = f
		cg.dispatchSync(cg.mainQueue, layout.block)
		layout.f = nil
		return
	}
	done := make(chan struct{})
	layout.work <- func() {
		defer close(done)
		f()
	}
	<-done
}

// readKeymap asks the current layout what each key types, plain and
// shifted. Keypad keys are left out so digits come from the main row.
func readKeymap() map[rune]macKey {
	keymap := map[rune]macKey{}
	src := cg.tisLayout()
	if src == 0 {
		return keymap
	}
	defer cg.cfRelease(src)
	data := cg.tisProperty(src, cg.tisLayoutDataKey)
	if data == 0 {
		return keymap
	}
	layout := cg.cfDataPtr(data)
	kbd := uint32(cg.kbdType())
	for _, shift := range []bool{false, true} {
		mods := uint32(0)
		if shift {
			mods = 0x02 // shiftKey >> 8
		}
		for code := uint16(0); code < 128; code++ {
			if code >= 65 && code <= 92 {
				continue
			}
			var dead uint32
			var n uintptr
			var buf [4]uint16
			// kUCKeyActionDisplay, kUCKeyTranslateNoDeadKeysMask
			if cg.keyTranslate(layout, code, 3, mods, kbd, 1, &dead, 4, &n, &buf[0]) != 0 || n != 1 {
				continue
			}
			r := rune(buf[0])
			if r < 0x20 || r == 0x7f {
				continue
			}
			if _, ok := keymap[r]; !ok {
				keymap[r] = macKey{code: code, shift: shift}
			}
		}
	}
	return keymap
}

func (q *quartz) changeCount() int64 {
	var n int64
	withPool(func() {
		n = int64(pasteboard().Send(selChangeCount))
	})
	return n
}

func pasteboard() objc.ID {
	return objc.ID(objc.GetClass("NSPasteboard")).Send(selGeneralPasteboard)
}

func (q *quartz) Clipboard() (string, error) {
	var text string
	withPool(func() {
		s := pasteboard().Send(selStringForType, cg.pasteboardStringKey)
		text = goString(s)
	})
	return text, nil
}

func (q *quartz) SetClipboard(text string) error {
	var ok bool
	withPool(func() {
		pb := pasteboard()
		pb.Send(selClearContents)
		ok = objc.Send[bool](pb, selSetStringForType, nsString(text), cg.pasteboardStringKey)
		q.clip.setOwn(int64(pb.Send(selChangeCount)))
	})
	if !ok {
		return errors.New("could not set the clipboard")
	}
	return nil
}

func (q *quartz) ClipboardChanges() (<-chan string, func()) { return q.clip.subscribe() }

func (q *quartz) Close() {
	cgCurrent.Lock()
	if cgCurrent.q == q {
		cgCurrent.q = nil
	}
	cgCurrent.Unlock()
	q.clip.close()
	q.inMu.Lock()
	// Whatever a viewer still holds is let go, as on X when the client goes.
	for ks := range q.mods {
		delete(q.mods, ks)
		if ev := cg.keyEvent(0, macModifiers[ks].code, false); ev != 0 {
			cg.setType(ev, flagsChanged)
			q.postLocked(ev, q.flagsLocked()) //nolint:errcheck
		}
	}
	q.inMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.stopLocked()
	if q.surface != 0 {
		releaseSurface(q.surface)
		q.surface = 0
	}
}

// withPool runs f inside an autorelease pool on one thread, so the
// Objective-C objects f is handed are freed when it returns.
func withPool(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := cg.poolPush()
	defer cg.poolPop(pool)
	f()
}

func nsString(s string) objc.ID {
	str := objc.ID(objc.GetClass("NSString")).Send(selAlloc)
	var p unsafe.Pointer
	if s != "" {
		p = unsafe.Pointer(unsafe.StringData(s))
	}
	str = str.Send(selInitWithBytesLengthEncoding, p, uintptr(len(s)), uintptr(4)) // UTF-8
	return str.Send(selAutorelease)
}

func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	n := int(s.Send(selLengthOfBytesUsingEncoding, uintptr(4)))
	p := objc.Send[unsafe.Pointer](s, selUTF8String)
	if p == nil || n == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(p), n))
}

func nsBool(b bool) objc.ID {
	return objc.ID(objc.GetClass("NSNumber")).Send(selNumberWithBool, b)
}

func nsDouble(f float64) objc.ID {
	return objc.ID(objc.GetClass("NSNumber")).Send(selNumberWithDouble, f)
}

func nsDictionary(keys, values []objc.ID) objc.ID {
	return objc.ID(objc.GetClass("NSDictionary")).Send(selDictionaryWithObjectsForKeysCount,
		unsafe.Pointer(&values[0]), unsafe.Pointer(&keys[0]), uintptr(len(keys)))
}

// macModifiers are the modifier keys, by keysym: Command is Super, as
// Linux and Windows keyboards label it.
var macModifiers = map[uint32]struct {
	code uint16
	flag uint64
}{
	0xffe1: {56, flagShift}, 0xffe2: {60, flagShift},
	0xffe3: {59, flagControl}, 0xffe4: {62, flagControl},
	0xffe9: {58, flagOption}, 0xffea: {61, flagOption},
	0xffeb: {55, flagCommand}, 0xffec: {54, flagCommand},
	0xffe7: {55, flagCommand}, 0xffe8: {54, flagCommand}, // Meta
}

// macSpecialKeys are the keys that type no character, by keysym.
var macSpecialKeys = map[uint32]uint16{
	0xff0d: 36, 0xff8d: 76, 0xff09: 48, 0xff08: 51, 0xff1b: 53, 0xffff: 117,
	0xff50: 115, 0xff57: 119, 0xff55: 116, 0xff56: 121,
	0xff51: 123, 0xff53: 124, 0xff54: 125, 0xff52: 126,
	0xff63: 114, 0xff67: 110, 0xffe5: 57, 0x20: 49,
	0x1008ff12: 74, 0x1008ff11: 73, 0x1008ff13: 72,
	// F1–F20
	0xffbe: 122, 0xffbf: 120, 0xffc0: 99, 0xffc1: 118, 0xffc2: 96, 0xffc3: 97,
	0xffc4: 98, 0xffc5: 100, 0xffc6: 101, 0xffc7: 109, 0xffc8: 103, 0xffc9: 111,
	0xffca: 105, 0xffcb: 107, 0xffcc: 113, 0xffcd: 106, 0xffce: 64, 0xffcf: 79,
	0xffd0: 80, 0xffd1: 90,
}
