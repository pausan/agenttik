package traypulse

import (
	"sync/atomic"
	"time"
)

// Step is how long one frame of the pulse is shown. Twelve steps make the
// 1.8s cycle. It is deliberately slow: every step is an icon the tray host has
// to be handed, over D-Bus on Linux and through the shell's icon cache on
// Windows, and a breath does not need more.
const Step = 150 * time.Millisecond

// Animator shows the resting icon while nothing is running and plays the
// pulse while something is. It owns every call to setIcon, so the tray is
// only ever touched from the one goroutine running Run.
type Animator struct {
	setIcon func([]byte)
	resting []byte
	frames  [][]byte

	step time.Duration
	busy atomic.Bool
	// wake carries "busy changed"; it holds one token, because what the loop
	// needs to know is that the flag moved, not how many times.
	wake chan struct{}
}

// New returns an animator for frames from Frames, played forwards then
// backwards. resting is the icon shown whenever nothing is running.
func New(setIcon func([]byte), resting []byte, frames [][]byte) *Animator {
	return &Animator{setIcon: setIcon, resting: resting, frames: frames,
		step: Step, wake: make(chan struct{}, 1)}
}

// SetBusy says whether any turn is in flight. It never blocks and may be
// called from any goroutine, including one holding a lock: the work it causes
// happens on Run's goroutine.
func (a *Animator) SetBusy(busy bool) {
	if a == nil {
		return
	}
	a.busy.Store(busy)
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Run drives the icon until done is closed, leaving the resting icon behind.
// With fewer than two frames it only ever shows the resting icon, so a failed
// render degrades to the tray as it was.
func (a *Animator) Run(done <-chan struct{}) {
	// The step of the cycle on the tray now: resting, or an index into the
	// ping-pong cycle. It starts at neither, so Run puts the resting icon up
	// itself and owns every icon the tray is given from then on.
	const resting, unset = -1, -2
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
		case running:
			running = false
			ticker.Stop()
			show(resting)
		default:
			show(resting)
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
