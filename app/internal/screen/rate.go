package screen

import (
	"math"
	"time"
)

// rateControl fits one viewer's stream to its link, the way a video call
// does: by watching queueing delay rather than loss, which a TCP link never
// shows. The viewer reports when each frame arrived on its own clock; less
// the time it was sent, that is the one-way delay plus an unknown, constant
// clock offset. The smallest value over the last ten seconds is the empty
// link, so anything above it is frames waiting in a queue somewhere.
//
// Queueing cuts the bitrate cap to what the viewer is actually receiving;
// a calm link raises it again. The frame rate follows the cap, so a slow link
// gets fewer, sharp frames rather than many blurred ones. See
// specs/096-screen.md.
type rateControl struct {
	kbps    float64 // the encoder's cap
	recvKbs float64 // what the viewer says it receives
	queue   time.Duration
	delays  []delaySample
	srtt    time.Duration

	calmSince, lastCut, lastGrow time.Time
	queueAtCut                   time.Duration
}

type delaySample struct {
	at    time.Time
	delay time.Duration
}

const (
	maxKbps   = 8000
	minKbps   = 100
	startKbps = 2000

	baseWindow   = 10 * time.Second // how long the empty-link delay is remembered
	congested    = 100 * time.Millisecond
	calm         = 50 * time.Millisecond
	calmFor      = 500 * time.Millisecond
	growEvery    = 200 * time.Millisecond
	growBy       = 1.06
	cutBy        = 0.85
	cutEvery     = 300 * time.Millisecond
	kbitPerFrame = 30 // the least a frame gets before the rate drops
	minFPS       = 5
)

func newRateControl(now time.Time) *rateControl {
	return &rateControl{kbps: startKbps, calmSince: now}
}

// received records that a frame sent at `sent` arrived at
// `arrived` on the viewer's clock, and what the viewer has been receiving.
func (r *rateControl) received(sent time.Time, arrived time.Duration, recvKbps float64, now time.Time) {
	if recvKbps > 0 {
		r.recvKbs = recvKbps
	}
	delay := arrived - time.Duration(sent.UnixNano())
	cut := 0
	for cut < len(r.delays) && now.Sub(r.delays[cut].at) > baseWindow {
		cut++
	}
	copy(r.delays, r.delays[cut:])
	r.delays = append(r.delays[:len(r.delays)-cut], delaySample{at: now, delay: delay})
	base := delay
	for _, s := range r.delays {
		base = min(base, s.delay)
	}
	r.queue = delay - base
	r.adjust(now)
}

// rtt records a round trip, from a frame's sending to its acknowledgement.
func (r *rateControl) rtt(d time.Duration) {
	if r.srtt == 0 {
		r.srtt = d
	} else {
		r.srtt = (7*r.srtt + d) / 8
	}
}

func (r *rateControl) adjust(now time.Time) {
	switch {
	case r.queue > congested:
		r.calmSince = now
		// A cut takes a queue's length to show. Until then, and while the
		// queue is draining, there is nothing new to react to.
		if now.Sub(r.lastCut) < cutEvery+r.queueAtCut || r.queue < r.queueAtCut {
			return
		}
		next := r.kbps * cutBy
		if r.recvKbs > 0 {
			next = min(next, r.recvKbs*0.9)
		}
		r.kbps = max(minKbps, next)
		r.lastCut, r.queueAtCut = now, r.queue
	case r.queue > calm:
		r.calmSince = now
	case r.queueAtCut > 0:
		r.queueAtCut = 0 // drained
	case now.Sub(r.calmSince) >= calmFor && now.Sub(r.lastGrow) >= growEvery:
		r.kbps = min(maxKbps, r.kbps*growBy)
		r.lastGrow = now
	}
}

// fps is how many frames a second the cap allows, each with a useful
// budget.
func (r *rateControl) fps() int {
	return int(math.Max(minFPS, math.Min(frameRate, r.kbps/kbitPerFrame)))
}

func (r *rateControl) interval() time.Duration { return time.Second / time.Duration(r.fps()) }

// window is how much may be unacknowledged: about a round trip of frames
// and of bytes at the cap, plus slack, so a long link is not starved and a
// slow one does not queue.
func (r *rateControl) window() (frames, bytes int) {
	span := r.srtt + 100*time.Millisecond
	frames = int(math.Ceil(float64(r.fps()) * span.Seconds()))
	bytes = int(r.kbps * 1000 / 8 * span.Seconds())
	return max(4, min(frames, 30)), max(48<<10, bytes)
}
