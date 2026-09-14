package traypulse

import (
	"sync"
	"testing"
	"time"
)

// recorder stands in for the tray: it keeps every icon it was handed, in order.
type recorder struct {
	mu   sync.Mutex
	seen []byte // one byte per icon, the frame's payload
}

func (r *recorder) set(icon []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, icon[0])
}

func (r *recorder) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.seen...)
}

func (r *recorder) waitFor(t *testing.T, want func([]byte) bool, msg string) []byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.snapshot(); want(got) {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s, saw %v", msg, r.snapshot())
	return nil
}

// testAnimator runs an animator whose frames are numbered 1..n, with the
// resting icon as 0, so the recorder's bytes read as the sequence shown.
func testAnimator(t *testing.T, frames int) (*Animator, *recorder, func()) {
	t.Helper()
	rec := &recorder{}
	pulse := make([][]byte, frames)
	for i := range pulse {
		pulse[i] = []byte{byte(i + 1)}
	}
	a := New(rec.set, []byte{0}, pulse)
	a.step = time.Millisecond

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { defer close(stopped); a.Run(done) }()
	return a, rec, func() {
		close(done)
		<-stopped
	}
}

func TestAnimatorPlaysTheCycleForwardsAndBack(t *testing.T) {
	a, rec, stop := testAnimator(t, 4)
	defer stop()

	// Resting until something runs.
	rec.waitFor(t, func(seen []byte) bool { return len(seen) > 0 }, "the resting icon")
	if got := rec.snapshot(); got[0] != 0 {
		t.Fatalf("started on frame %d, want the resting icon", got[0])
	}

	a.SetBusy(true)
	// 4 frames fold into a 6-step cycle: 1 2 3 4 3 2, then round again.
	seen := rec.waitFor(t, func(seen []byte) bool { return len(seen) >= 1+8 }, "a full cycle")
	want := []byte{0, 1, 2, 3, 4, 3, 2, 1, 2}
	for i, w := range want {
		if seen[i] != w {
			t.Fatalf("step %d showed frame %d, want %d (saw %v)", i, seen[i], w, seen[:len(want)])
		}
	}
}

func TestAnimatorRestsWhenWorkStops(t *testing.T) {
	a, rec, stop := testAnimator(t, 4)
	defer stop()

	a.SetBusy(true)
	rec.waitFor(t, func(seen []byte) bool { return len(seen) >= 4 }, "the pulse to start")
	a.SetBusy(false)
	seen := rec.waitFor(t, func(seen []byte) bool { return seen[len(seen)-1] == 0 }, "the resting icon")

	// And it stays there: no ticking while idle.
	time.Sleep(20 * a.step)
	if after := rec.snapshot(); len(after) != len(seen) {
		t.Fatalf("kept changing the icon while idle: %v", after[len(seen):])
	}

	// The next spell of work starts from the dimmest frame again.
	a.SetBusy(true)
	next := rec.waitFor(t, func(s []byte) bool { return len(s) > len(seen) }, "the pulse to start again")
	if got := next[len(seen)]; got != 1 {
		t.Fatalf("resumed on frame %d, want the dimmest", got)
	}
}

func TestAnimatorLeavesTheRestingIconOnStop(t *testing.T) {
	a, rec, stop := testAnimator(t, 4)
	a.SetBusy(true)
	rec.waitFor(t, func(seen []byte) bool { return len(seen) >= 4 }, "the pulse to start")
	stop()

	seen := rec.snapshot()
	if last := seen[len(seen)-1]; last != 0 {
		t.Fatalf("stopped showing frame %d, want the resting icon", last)
	}
}

// A render that failed leaves the tray with what it had, rather than a pulse
// that cannot be played.
func TestAnimatorWithoutFramesStaysResting(t *testing.T) {
	a, rec, stop := testAnimator(t, 0)
	defer stop()

	a.SetBusy(true)
	time.Sleep(20 * a.step)
	for i, icon := range rec.waitFor(t, func(seen []byte) bool { return len(seen) > 0 }, "the resting icon") {
		if icon != 0 {
			t.Fatalf("step %d showed frame %d, want the resting icon throughout", i, icon)
		}
	}
}
