package screen

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openAudio records what the default output plays with WASAPI loopback,
// through COM's vtables with no cgo. The audio engine converts its mix to
// 48 kHz 16-bit stereo for us (AUTOCONVERTPCM).
var openAudio = openWASAPI

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidMMDeviceEnumerator = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient         = windows.GUID{Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4C32, Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCaptureClient  = windows.GUID{Data1: 0xC8ADBD64, Data2: 0xE71E, Data3: 0x48A0, Data4: [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
)

// Vtable slots, counting IUnknown's QueryInterface, AddRef and Release.
const (
	comRelease               = 2
	enumGetDefaultEndpoint   = 4 // IMMDeviceEnumerator
	deviceActivate           = 3 // IMMDevice
	clientInitialize         = 3 // IAudioClient
	clientStart              = 10
	clientStop               = 11
	clientGetService         = 14
	captureGetBuffer         = 3 // IAudioCaptureClient
	captureReleaseBuffer     = 4
	captureGetNextPacketSize = 5

	clsctxAll               = 0x17
	eRender                 = 0
	eConsole                = 0
	sharedMode              = 0
	streamLoopback          = 0x00020000
	streamAutoConvertPCM    = 0x80000000
	streamSrcDefaultQuality = 0x08000000
	bufferSilent            = 0x2
	waveFormatPCM           = 1
	bufferDuration          = 200 * 10000 // 200 ms in 100 ns units
	pollInterval            = 10 * time.Millisecond
)

// comObject is a COM interface pointer: its first word is the vtable.
type comObject struct{ vtbl *[16]uintptr }

func (o *comObject) call(slot int, args ...uintptr) error {
	r, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	if int32(r) < 0 {
		return fmt.Errorf("HRESULT 0x%08X", uint32(r))
	}
	return nil
}

func (o *comObject) release() {
	if o != nil {
		o.call(comRelease) //nolint:errcheck
	}
}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

type wasapiAudio struct {
	buf  *pcmBuffer
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// openWASAPI starts capture on a thread of its own, where COM is
// initialised, and answers once the first device is open.
func openWASAPI() (Audio, error) {
	if err := procCoCreateInstance.Find(); err != nil {
		return nil, fmt.Errorf("no Windows audio: %w", err)
	}
	a := &wasapiAudio{buf: newPCMBuffer(), stop: make(chan struct{}), done: make(chan struct{})}
	opened := make(chan error, 1)
	go a.run(opened)
	if err := <-opened; err != nil {
		<-a.done
		return nil, err
	}
	return a, nil
}

func (a *wasapiAudio) run(opened chan<- error) {
	defer close(a.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		opened <- fmt.Errorf("initialise COM: %w", err)
		return
	}
	defer windows.CoUninitialize()
	first := true
	for {
		client, capture, err := openLoopback()
		if first {
			opened <- err
			if err != nil {
				return
			}
			first = false
		}
		if err == nil {
			err = a.drain(capture)
			client.call(clientStop) //nolint:errcheck
			capture.release()
			client.release()
		}
		if err == nil {
			return // closed
		}
		// The output was unplugged, or another became the default: wait a
		// moment and record whatever is the default now.
		select {
		case <-a.stop:
			return
		case <-time.After(time.Second):
		}
	}
}

// openLoopback opens the default output for loopback capture and starts it.
func openLoopback() (client, capture *comObject, err error) {
	var enum, device *comObject
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	if int32(r) < 0 {
		return nil, nil, fmt.Errorf("no audio devices: HRESULT 0x%08X", uint32(r))
	}
	defer enum.release()
	if err := enum.call(enumGetDefaultEndpoint, eRender, eConsole, uintptr(unsafe.Pointer(&device))); err != nil {
		return nil, nil, fmt.Errorf("no sound output: %w", err)
	}
	defer device.release()
	if err := device.call(deviceActivate, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))); err != nil {
		return nil, nil, fmt.Errorf("open the sound output: %w", err)
	}
	format := waveFormatEx{FormatTag: waveFormatPCM, Channels: 2, SamplesPerSec: 48000, AvgBytesPerSec: 48000 * 4, BlockAlign: 4, BitsPerSample: 16}
	flags := uintptr(streamLoopback | streamAutoConvertPCM | streamSrcDefaultQuality)
	if err := client.call(clientInitialize, sharedMode, flags, bufferDuration, 0, uintptr(unsafe.Pointer(&format)), 0); err != nil {
		client.release()
		return nil, nil, fmt.Errorf("record the sound output: %w", err)
	}
	if err := client.call(clientGetService, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capture))); err != nil {
		client.release()
		return nil, nil, fmt.Errorf("record the sound output: %w", err)
	}
	if err := client.call(clientStart); err != nil {
		capture.release()
		client.release()
		return nil, nil, fmt.Errorf("start recording the sound output: %w", err)
	}
	return client, capture, nil
}

// drain moves packets into the buffer until closed (nil) or the device
// fails. Loopback delivers nothing while nothing plays.
func (a *wasapiAudio) drain(capture *comObject) error {
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-a.stop:
			return nil
		case <-tick.C:
		}
		for {
			var size uint32
			if err := capture.call(captureGetNextPacketSize, uintptr(unsafe.Pointer(&size))); err != nil {
				return err
			}
			if size == 0 {
				break
			}
			var data *int16
			var frames, flags uint32
			if err := capture.call(captureGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
				uintptr(unsafe.Pointer(&flags)), 0, 0); err != nil {
				return err
			}
			pcm := make([]int16, 2*frames)
			if flags&bufferSilent == 0 && data != nil && frames > 0 {
				copy(pcm, unsafe.Slice(data, 2*frames))
			}
			if err := capture.call(captureReleaseBuffer, uintptr(frames)); err != nil {
				return err
			}
			a.buf.write(pcm)
		}
	}
}

func (a *wasapiAudio) Read(pcm []int16) error { return a.buf.read(pcm, nil) }

func (a *wasapiAudio) Close() {
	a.once.Do(func() {
		close(a.stop)
		<-a.done
		a.buf.fail(errAudioClosed)
	})
}
