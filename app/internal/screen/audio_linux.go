package screen

import (
	"fmt"
	"os"
	"sync"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// openAudio records what the default output plays, from its monitor source,
// through the PulseAudio protocol, which PipeWire also speaks. Nothing is
// linked: the protocol is spoken from Go. PULSE_SOURCE names another source,
// as it does for libpulse.
var openAudio = func() (Audio, error) {
	if source := os.Getenv("PULSE_SOURCE"); source != "" {
		return openPulse(source)
	}
	return openPulse("@DEFAULT_MONITOR@")
}

type pulseAudio struct {
	client *pulse.Client
	stream *pulse.RecordStream
	buf    *pcmBuffer
	once   sync.Once
}

// openPulse records the named source. The default output's monitor goes by
// name rather than by one sink's: it is whatever plays now.
func openPulse(source string) (Audio, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("agenttik"))
	if err != nil {
		return nil, fmt.Errorf("no sound server: %w", err)
	}
	a := &pulseAudio{client: client, buf: newPCMBuffer()}
	a.stream, err = client.NewRecord(pulse.Int16Writer(a.write),
		pulse.RecordStereo, pulse.RecordSampleRate(48000), pulse.RecordLatency(0.02),
		pulse.RecordMediaName("Screen sharing"),
		pulse.RecordRawOption(func(r *proto.CreateRecordStream) { r.SourceName = source }))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("record the sound: %w", err)
	}
	if a.stream.SampleRate() != 48000 || a.stream.Channels() != 2 {
		a.Close()
		return nil, fmt.Errorf("the sound server gives %d Hz, %d channels", a.stream.SampleRate(), a.stream.Channels())
	}
	a.stream.Start()
	return a, nil
}

func (a *pulseAudio) write(pcm []int16) (int, error) {
	a.buf.write(pcm)
	return len(pcm), nil
}

func (a *pulseAudio) Read(pcm []int16) error {
	return a.buf.read(pcm, func() error {
		if a.stream.Closed() {
			return fmt.Errorf("the sound server went away")
		}
		return nil
	})
}

func (a *pulseAudio) Close() {
	a.once.Do(func() {
		a.buf.fail(errAudioClosed)
		a.stream.Close()
		a.client.Close()
	})
}
