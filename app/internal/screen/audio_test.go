package screen

import (
	"errors"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/screen/vpx"
)

func audioFrame(level int16) []int16 {
	pcm := make([]int16, 2*vpx.OpusFrame)
	for i := range pcm {
		if i%4 < 2 {
			pcm[i] = level
		} else {
			pcm[i] = -level
		}
	}
	return pcm
}

func TestViewerSendsSoundButNotSilence(t *testing.T) {
	if err := vpx.Load(); err != nil {
		t.Skip(err)
	}
	m, _ := fakeManager(t, 64, 64)
	a := NewFakeAudio()
	m.UseAudio(func() (Audio, error) { return a, nil })
	if !m.Info().Audio {
		t.Fatal("info does not offer sound")
	}
	v, err := m.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	v.SetAudio(true)

	sound, silence := audioFrame(3000), audioFrame(0)
	for _, f := range [][]int16{sound, sound, silence, sound, sound, sound, silence} {
		a.Play(f)
	}
	// Packets are numbered by the 20 ms they cover, so the silent third is
	// a gap, and silence sends what was waiting for its batch.
	for _, want := range []struct {
		seq uint64
		n   int
	}{{1, 2}, {4, 2}, {6, 1}} {
		msg := next(t, v, "audio")
		if msg.Seq != want.seq || len(msg.Packets) != want.n || msg.Text != "" {
			t.Fatalf("audio %d with %d packets (%q), want %d with %d", msg.Seq, len(msg.Packets), msg.Text, want.seq, want.n)
		}
	}

	v.SetAudio(false)
	select {
	case <-a.Closed():
	case <-time.After(time.Second):
		t.Fatal("turning sound off left the capture open")
	}
}

func TestViewerClosesWhileSoundPlays(t *testing.T) {
	if err := vpx.Load(); err != nil {
		t.Skip(err)
	}
	m, _ := fakeManager(t, 64, 64)
	a := NewFakeAudio()
	m.UseAudio(func() (Audio, error) { return a, nil })
	v, err := m.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	v.SetAudio(true)
	for range 10 {
		a.Play(audioFrame(3000))
	}
	v.Close() // nobody reads the stream; this must not hang or panic
	select {
	case <-a.Closed():
	case <-time.After(time.Second):
		t.Fatal("closing the viewer left the capture open")
	}
	v.SetAudio(true) // too late: nothing starts
}

func TestViewerSaysWhyThereIsNoSound(t *testing.T) {
	m, _ := fakeManager(t, 64, 64)
	m.UseAudio(func() (Audio, error) { return nil, errors.New("no sound server") })
	v, err := m.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	v.SetAudio(true)
	if msg := next(t, v, "audio"); msg.Text != "no sound server" || len(msg.Packets) != 0 {
		t.Fatalf("audio %+v", msg)
	}

	m.UseAudio(nil)
	if m.Info().Audio {
		t.Fatal("info offers sound with no way to record it")
	}
}

func TestPCMBufferKeepsTheNewestSecond(t *testing.T) {
	b := newPCMBuffer()
	b.write(make([]int16, pcmKeep))
	b.write([]int16{1, 2, 3, 4})
	if len(b.buf) != pcmKeep || b.buf[len(b.buf)-1] != 4 {
		t.Fatalf("kept %d samples ending %d", len(b.buf), b.buf[len(b.buf)-1])
	}
	pcm := make([]int16, pcmKeep)
	if err := b.read(pcm, nil); err != nil || pcm[pcmKeep-4] != 1 {
		t.Fatalf("read %v, sample %d", err, pcm[pcmKeep-4])
	}
	b.write([]int16{5, 6})
	b.fail(errAudioClosed)
	if err := b.read(pcm[:4], nil); err != errAudioClosed {
		t.Fatalf("short read after fail: %v", err)
	}
}
