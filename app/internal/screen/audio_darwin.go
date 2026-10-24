package screen

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// openAudio records what the Mac plays with ScreenCaptureKit (macOS 13 and
// later), whose streams carry the system's sound beside the screen. It is
// allowed by the same Screen & System Audio Recording permission as the
// screen. agenttik's own sound is left out, so a Mac viewing another
// machine never sends that machine its own sound back.
var openAudio = openSCK

type sckAudio struct {
	buf     *pcmBuffer
	stream  objc.ID
	handler objc.ID
	once    sync.Once
}

// audioBufferList is CoreAudio's AudioBufferList with room for two
// buffers, one per channel when the samples are not interleaved.
type audioBufferList struct {
	Count   uint32
	_       uint32
	Buffers [2]struct {
		Channels uint32
		Size     uint32
		Data     unsafe.Pointer
	}
}

// audioStreamBasicDescription is CoreAudio's description of a format.
type audioStreamBasicDescription struct {
	SampleRate       float64
	FormatID         uint32
	FormatFlags      uint32
	BytesPerPacket   uint32
	FramesPerPacket  uint32
	BytesPerFrame    uint32
	ChannelsPerFrame uint32
	BitsPerChannel   uint32
	Reserved         uint32
}

const (
	scOutputScreen     = 0
	scOutputAudio      = 1
	formatIsFloat      = 1 << 0
	formatNonInterlvd  = 1 << 5
	ablAssureAlignment = 1 << 0
)

var (
	sck struct {
		once     sync.Once
		err      error
		openMu   sync.Mutex // one stream is set up at a time; the replies share channels
		queue    uintptr
		handler  objc.Class
		content  objc.Block
		done     objc.Block
		contents chan [2]objc.ID
		replies  chan objc.ID

		bufferList func(sbuf uintptr, sizeNeeded unsafe.Pointer, abl unsafe.Pointer, size uintptr, a, b uintptr, flags uint32, block *uintptr) int32
		samples    func(sbuf uintptr) int64
		format     func(sbuf uintptr) uintptr
		basic      func(desc uintptr) unsafe.Pointer
	}
	// sckCaptures finds the capture a handler object feeds.
	sckCaptures sync.Map // objc.ID → *sckAudio

	selDisplays                = objc.RegisterName("displays")
	selDisplayID               = objc.RegisterName("displayID")
	selCount                   = objc.RegisterName("count")
	selObjectAtIndex           = objc.RegisterName("objectAtIndex:")
	selArray                   = objc.RegisterName("array")
	selInit                    = objc.RegisterName("init")
	selRetain                  = objc.RegisterName("retain")
	selRelease                 = objc.RegisterName("release")
	selLocalizedDescription    = objc.RegisterName("localizedDescription")
	selGetShareableContent     = objc.RegisterName("getShareableContentWithCompletionHandler:")
	selInitWithDisplay         = objc.RegisterName("initWithDisplay:excludingWindows:")
	selSetCapturesAudio        = objc.RegisterName("setCapturesAudio:")
	selSetSampleRate           = objc.RegisterName("setSampleRate:")
	selSetChannelCount         = objc.RegisterName("setChannelCount:")
	selSetExcludesCurrentAudio = objc.RegisterName("setExcludesCurrentProcessAudio:")
	selSetWidth                = objc.RegisterName("setWidth:")
	selSetHeight               = objc.RegisterName("setHeight:")
	selInitWithFilter          = objc.RegisterName("initWithFilter:configuration:delegate:")
	selAddStreamOutput         = objc.RegisterName("addStreamOutput:type:sampleHandlerQueue:error:")
	selStartCapture            = objc.RegisterName("startCaptureWithCompletionHandler:")
	selStopCapture             = objc.RegisterName("stopCaptureWithCompletionHandler:")
	selStreamDidOutput         = objc.RegisterName("stream:didOutputSampleBuffer:ofType:")
)

func loadSCK() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	cgOnce.Do(func() { cgErr = loadQuartz() })
	if cgErr != nil {
		return cgErr
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return err
	}
	cm, err := purego.Dlopen("/System/Library/Frameworks/CoreMedia.framework/CoreMedia", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return err
	}
	if objc.GetClass("SCStream") == 0 {
		return errors.New("no ScreenCaptureKit")
	}
	purego.RegisterLibFunc(&sck.bufferList, cm, "CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer")
	purego.RegisterLibFunc(&sck.samples, cm, "CMSampleBufferGetNumSamples")
	purego.RegisterLibFunc(&sck.format, cm, "CMSampleBufferGetFormatDescription")
	purego.RegisterLibFunc(&sck.basic, cm, "CMAudioFormatDescriptionGetStreamBasicDescription")

	var protocols []*objc.Protocol
	if p := objc.GetProtocol("SCStreamOutput"); p != nil {
		protocols = append(protocols, p)
	}
	sck.handler, err = objc.RegisterClass("AgenttikSoundOutput", objc.GetClass("NSObject"), protocols, nil, []objc.MethodDef{{
		Cmd: selStreamDidOutput,
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, sbuf uintptr, kind int) {
			if kind != scOutputAudio {
				return
			}
			if a, ok := sckCaptures.Load(self); ok {
				a.(*sckAudio).sample(sbuf)
			}
		},
	}})
	if err != nil {
		return err
	}
	// Blocks are made once: purego never frees their callbacks.
	sck.contents = make(chan [2]objc.ID, 1)
	sck.replies = make(chan objc.ID, 1)
	sck.content = objc.NewBlock(func(_ objc.Block, content, nserr objc.ID) {
		if content != 0 {
			content.Send(selRetain)
		}
		if nserr != 0 {
			nserr.Send(selRetain)
		}
		sck.contents <- [2]objc.ID{content, nserr}
	})
	sck.done = objc.NewBlock(func(_ objc.Block, nserr objc.ID) {
		if nserr != 0 {
			nserr.Send(selRetain)
		}
		sck.replies <- nserr
	})
	name := []byte("agenttik.sound\x00")
	sck.queue = cg.queueCreate(&name[0], 0)
	return nil
}

func openSCK() (Audio, error) {
	sck.once.Do(func() { sck.err = loadSCK() })
	if sck.err != nil {
		return nil, fmt.Errorf("sound needs macOS 13 or later: %w", sck.err)
	}
	if !cg.preflightCapture() {
		return nil, errors.New("macOS has not allowed agenttik to record the screen and its sound. Allow it in System Settings › Privacy & Security › Screen & System Audio Recording, then restart agenttik.")
	}
	sck.openMu.Lock()
	defer sck.openMu.Unlock()
	sckDrain()

	var content objc.ID
	withPool(func() {
		objc.ID(objc.GetClass("SCShareableContent")).Send(selGetShareableContent, sck.content)
	})
	select {
	case reply := <-sck.contents:
		content = reply[0]
		if reply[1] != 0 {
			defer reply[1].Send(selRelease)
			if content != 0 {
				content.Send(selRelease)
			}
			return nil, fmt.Errorf("list the displays: %s", errorText(reply[1]))
		}
	case <-time.After(5 * time.Second):
		return nil, errors.New("macOS did not list the displays")
	}
	if content == 0 {
		return nil, errors.New("macOS listed no displays")
	}
	defer content.Send(selRelease)

	a := &sckAudio{buf: newPCMBuffer()}
	var err error
	withPool(func() {
		display := mainSCDisplay(content)
		if display == 0 {
			err = errors.New("no display to record the sound of")
			return
		}
		empty := objc.ID(objc.GetClass("NSArray")).Send(selArray)
		filter := objc.ID(objc.GetClass("SCContentFilter")).Send(selAlloc).Send(selInitWithDisplay, display, empty)
		defer filter.Send(selRelease)
		config := objc.ID(objc.GetClass("SCStreamConfiguration")).Send(selAlloc).Send(selInit)
		defer config.Send(selRelease)
		config.Send(selSetCapturesAudio, true)
		config.Send(selSetSampleRate, 48000)
		config.Send(selSetChannelCount, 2)
		config.Send(selSetExcludesCurrentAudio, true)
		// The picture is not wanted, only the sound beside it.
		config.Send(selSetWidth, uintptr(64))
		config.Send(selSetHeight, uintptr(64))
		a.stream = objc.ID(objc.GetClass("SCStream")).Send(selAlloc).Send(selInitWithFilter, filter, config, objc.ID(0))
		a.handler = objc.ID(sck.handler).Send(selAlloc).Send(selInit)
		if a.stream == 0 || a.handler == 0 {
			err = errors.New("macOS would not record the sound")
			return
		}
		sckCaptures.Store(a.handler, a)
		// The picture goes to the same handler, which drops it, so macOS
		// does not complain of a stream with no screen output.
		for _, kind := range []int{scOutputScreen, scOutputAudio} {
			if !objc.Send[bool](a.stream, selAddStreamOutput, a.handler, kind, sck.queue, uintptr(0)) {
				err = errors.New("macOS would not hand over the sound")
				return
			}
		}
		a.stream.Send(selStartCapture, sck.done)
	})
	if err == nil {
		select {
		case nserr := <-sck.replies:
			if nserr != 0 {
				err = fmt.Errorf("record the sound: %s", errorText(nserr))
				nserr.Send(selRelease)
			}
		case <-time.After(5 * time.Second):
			err = errors.New("macOS did not start recording the sound")
		}
	}
	if err != nil {
		a.release()
		return nil, err
	}
	return a, nil
}

// sckDrain empties the reply channels of answers that came after their wait.
func sckDrain() {
	for {
		select {
		case r := <-sck.contents:
			for _, id := range r {
				if id != 0 {
					id.Send(selRelease)
				}
			}
		case r := <-sck.replies:
			if r != 0 {
				r.Send(selRelease)
			}
		default:
			return
		}
	}
}

// mainSCDisplay is the main display among the shareable ones, else the first.
func mainSCDisplay(content objc.ID) objc.ID {
	displays := content.Send(selDisplays)
	n := int(objc.Send[uintptr](displays, selCount))
	if n == 0 {
		return 0
	}
	main := cg.mainDisplayID()
	for i := 0; i < n; i++ {
		d := displays.Send(selObjectAtIndex, uintptr(i))
		if objc.Send[uint32](d, selDisplayID) == main {
			return d
		}
	}
	return displays.Send(selObjectAtIndex, uintptr(0))
}

func errorText(nserr objc.ID) string {
	var s string
	withPool(func() { s = goString(nserr.Send(selLocalizedDescription)) })
	return s
}

// sample converts one buffer of sound, float samples whether interleaved or
// one buffer per channel, into 16-bit stereo. It runs on the stream's queue.
func (a *sckAudio) sample(sbuf uintptr) {
	n := int(sck.samples(sbuf))
	desc := (*audioStreamBasicDescription)(sck.basic(sck.format(sbuf)))
	if n <= 0 || desc == nil || desc.FormatFlags&formatIsFloat == 0 || desc.BitsPerChannel != 32 || desc.SampleRate != 48000 {
		return
	}
	var abl audioBufferList
	var block uintptr
	if sck.bufferList(sbuf, nil, unsafe.Pointer(&abl), unsafe.Sizeof(abl), 0, 0, ablAssureAlignment, &block) != 0 {
		return
	}
	defer cg.cfRelease(block)
	var left, right []float32
	step := 1
	switch {
	case abl.Count >= 2 && abl.Buffers[0].Size >= uint32(4*n) && abl.Buffers[1].Size >= uint32(4*n):
		left = unsafe.Slice((*float32)(abl.Buffers[0].Data), n)
		right = unsafe.Slice((*float32)(abl.Buffers[1].Data), n)
	case abl.Count == 1 && abl.Buffers[0].Channels > 0 && abl.Buffers[0].Size >= uint32(4*n)*abl.Buffers[0].Channels:
		step = int(abl.Buffers[0].Channels)
		all := unsafe.Slice((*float32)(abl.Buffers[0].Data), n*step)
		left, right = all, all
		if step > 1 {
			right = all[1:]
		}
	default:
		return
	}
	pcm := make([]int16, 2*n)
	for i := 0; i < n; i++ {
		pcm[2*i] = toInt16(left[i*step])
		pcm[2*i+1] = toInt16(right[i*step])
	}
	a.buf.write(pcm)
}

func toInt16(f float32) int16 {
	return int16(math.Round(float64(max(-1, min(1, f)) * 32767)))
}

func (a *sckAudio) Read(pcm []int16) error { return a.buf.read(pcm, nil) }

func (a *sckAudio) Close() {
	a.once.Do(func() {
		a.buf.fail(errAudioClosed)
		sck.openMu.Lock()
		defer sck.openMu.Unlock()
		sckDrain()
		a.stream.Send(selStopCapture, sck.done)
		select {
		case nserr := <-sck.replies:
			if nserr != 0 {
				nserr.Send(selRelease)
			}
		case <-time.After(2 * time.Second):
		}
		a.release()
	})
}

func (a *sckAudio) release() {
	if a.handler != 0 {
		sckCaptures.Delete(a.handler)
	}
	if a.stream != 0 {
		a.stream.Send(selRelease)
		a.stream = 0
	}
	if a.handler != 0 {
		a.handler.Send(selRelease)
		a.handler = 0
	}
}
