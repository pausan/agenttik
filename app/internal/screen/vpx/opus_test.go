package vpx

import (
	"math"
	"testing"
)

func TestOpusEncodesTone(t *testing.T) {
	needLibrary(t)
	o, err := NewOpus()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	pcm := make([]int16, 2*OpusFrame)
	total := 0
	for f := 0; f < 50; f++ {
		for i := 0; i < OpusFrame; i++ {
			v := int16(8000 * math.Sin(2*math.Pi*440*float64(f*OpusFrame+i)/48000))
			pcm[2*i], pcm[2*i+1] = v, v
		}
		pkt, err := o.Encode(pcm)
		if err != nil || len(pkt) == 0 {
			t.Fatalf("frame %d: %d bytes, %v", f, len(pkt), err)
		}
		total += len(pkt)
	}
	// One second of a tone lands near the bitrate it was given.
	if kbps := total * 8 / 1000; kbps < OpusKbps/2 || kbps > OpusKbps*2 {
		t.Fatalf("a second of tone took %d kbit, want about %d", kbps, OpusKbps)
	}
	if _, err := o.Encode(pcm[:10]); err == nil {
		t.Fatal("a short frame was accepted")
	}
}
