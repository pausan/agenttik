package traypulse

import (
	"sync/atomic"
	"time"
)

// Step is how long one frame of the pulse is shown. Twelve steps make the
// three-second cycle. It is deliberately slow: every step is an icon the tray
// host has to be handed, over D-Bus on Linux and through the shell's icon
// cache on Windows, and a breath does not need more.
const Step = 250 * time.Millisecond

// Animator plays the local pulse, shows a static remote badge for remote-only
// work, and rests when idle. It owns every call to setIcon on Run's goroutine.
type Animator struct {
	setIcon func([]byte)
	resting []byte
	remote  []byte
	frames  [][]byte

	step       time.Duration
	busy       atomic.Bool
	remoteBusy atomic.Bool
	// wake carries "busy changed"; it holds one token, because what the loop
	// needs to know is that the flag moved, not how many times.
	wake chan struct{}
}

// New returns an animator for frames from Frames, played forwards then
// backwards. remote is the icon for remote-only work; nil disables the badge.
func New(setIcon func([]byte), resting []byte, frames [][]byte, remote []byte) *Animator {
	return &Animator{setIcon: setIcon, resting: resting, frames: frames, remote: remote,
		step: Step, wake: make(chan struct{}, 1)}
}

// SetBusy says whether any local turn is in flight. It never blocks and may be
// called from any goroutine, including one holding a lock: the work it causes
// happens on Run's goroutine.
func (a *Animator) SetBusy(busy bool) {
	if a == nil {
		return
	}
	a.busy.Store(busy)
	a.wakeUp()
}

// SetRemoteBusy shows the Wi-Fi icon only while local work is idle.
func (a *Animator) SetRemoteBusy(busy bool) {
	if a == nil {
		return
	}
	a.remoteBusy.Store(busy)
	a.wakeUp()
}

func (a *Animator) wakeUp() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Run drives the icon until done is closed, leaving the resting icon behind.
// With fewer than two frames local work leaves the resting icon showing.
func (a *Animator) Run(done <-chan struct{}) {
	// The step of the cycle on the tray now: resting, or an index into the
	// ping-pong cycle. It starts at neither, so Run puts the resting icon up
	// itself and owns every icon the tray is given from then on.
	const resting, unset, remote = -1, -2, -3
	shown := unset
	show := func(i int) {
		if i == shown {
			return
		}
		shown = i
		if i == resting {
			a.setIcon(a.resting)
			return
		}
		if i == remote {
			a.setIcon(a.remote)
			return
		}
		a.setIcon(a.frames[a.frameAt(i)])
	}

	ticker := time.NewTicker(a.step)
	ticker.Stop()
	defer ticker.Stop()
	running := false

	for {
		switch {
		case a.busy.Load() && len(a.frames) >= 2:
			if !running {
				// Start at the dimmest frame rather than wherever the last
				// run stopped, so every spell of work begins the same way.
				running = true
				show(0)
				ticker.Reset(a.step)
			}
		default:
			if running {
				running = false
				ticker.Stop()
			}
			if !a.busy.Load() && a.remoteBusy.Load() && len(a.remote) > 0 {
				show(remote)
			} else {
				show(resting)
			}
		}

		select {
		case <-done:
			if shown != resting {
				a.setIcon(a.resting)
			}
			return
		case <-a.wake:
		case <-ticker.C:
			if running {
				show((shown + 1) % a.cycle())
			}
		}
	}
}

// cycle is how many steps a full pulse takes: up through the frames and back
// down, without repeating either end.
func (a *Animator) cycle() int { return 2*len(a.frames) - 2 }

// frameAt maps a step of the cycle onto a frame, folding the second half back
// over the first.
func (a *Animator) frameAt(step int) int {
	if step < len(a.frames) {
		return step
	}
	return a.cycle() - step
}
