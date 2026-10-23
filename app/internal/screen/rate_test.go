package screen

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/screen/vpx"
)

// feed reports a frame every 33 ms for d, each arriving with the delay
// delayAt gives it, the viewer receiving recv kbit/s.
func feed(r *rateControl, start time.Time, d time.Duration, recv float64, delayAt func(time.Duration) time.Duration) time.Time {
	now := start
	for t := time.Duration(0); t < d; t += 33 * time.Millisecond {
		now = start.Add(t)
		// The viewer's clock is offset by an hour: only differences matter.
		arrived := time.Duration(now.UnixNano()) + time.Hour + delayAt(t)
		r.received(now, arrived, recv, now)
	}
	return now
}

func TestRateGrowsOnACalmLink(t *testing.T) {
	start := time.Now()
	r := newRateControl(start)
	feed(r, start, 10*time.Second, 0, func(time.Duration) time.Duration { return 40 * time.Millisecond })
	if r.kbps != maxKbps || r.fps() != frameRate {
		t.Fatalf("after 10 calm seconds: %.0f kbps, %d fps", r.kbps, r.fps())
	}
}

func TestRateCutsToWhatArrivesWhenQueuesGrow(t *testing.T) {
	start := time.Now()
	r := newRateControl(start)
	now := feed(r, start, 2*time.Second, 0, func(time.Duration) time.Duration { return 40 * time.Millisecond })
	before := r.kbps
	// Each frame waits 10 ms longer than the last: a link full to the brim.
	feed(r, now, time.Second, 600, func(t time.Duration) time.Duration { return 40*time.Millisecond + t/3 })
	if r.kbps > 600*0.9 || r.kbps < 300 {
		t.Fatalf("queueing link: %.0f kbps (was %.0f), receiving 600", r.kbps, before)
	}
	if fps := r.fps(); fps > 18 || fps < minFPS {
		t.Fatalf("%.0f kbps gives %d fps", r.kbps, fps)
	}
}

func TestRateFramesAndWindow(t *testing.T) {
	r := newRateControl(time.Now())
	for kbps, fps := range map[float64]int{8000: 30, 900: 30, 600: 20, 300: 10, 100: 5} {
		r.kbps = kbps
		if r.fps() != fps {
			t.Errorf("%.0f kbps: %d fps, want %d", kbps, r.fps(), fps)
		}
	}
	r.kbps = 3000
	r.rtt(300 * time.Millisecond)
	if frames, bytes := r.window(); frames != 12 || bytes != 150000 {
		t.Errorf("300 ms round trip at 3 Mbit/s: %d frames, %d bytes", frames, bytes)
	}
}

func TestRateRecoversAfterTheQueueDrains(t *testing.T) {
	start := time.Now()
	r := newRateControl(start)
	r.received(start, time.Duration(start.UnixNano())+time.Hour+40*time.Millisecond, 0, start)
	r.received(start, time.Duration(start.UnixNano())+time.Hour+300*time.Millisecond, 600, start.Add(time.Second))
	if r.kbps != 540 {
		t.Fatalf("congested cap %.0f", r.kbps)
	}
	feed(r, start.Add(2*time.Second), 12*time.Second, 1000, func(time.Duration) time.Duration { return 40 * time.Millisecond })
	if r.kbps != maxKbps {
		t.Fatalf("recovered cap %.0f", r.kbps)
	}
	if len(r.delays) > 305 {
		t.Fatalf("kept %d delay samples past the ten-second window", len(r.delays))
	}
}

// TestRateFitsASlowLink streams a busy screen through a simulated 1 Mbit/s
// link with a 50 ms delay each way and a queue, as a slow connection behaves.
// The stream must settle under the link's rate with short queues, rather
// than fill them.
func TestRateFitsASlowLink(t *testing.T) {
	if vpx.Load() != nil {
		t.Skip("needs the VP9 library")
	}
	const link = 1e6 // bit/s
	const oneWay = 50 * time.Millisecond
	m, f := fakeManager(t, 640, 400)
	v, err := m.Watch("vp9")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// A screen where something moves everywhere, 30 times a second.
	stop := make(chan struct{})
	motionDone := make(chan struct{})
	defer func() { close(stop); <-motionDone }()
	go func() {
		defer close(motionDone)
		rng := rand.New(rand.NewPCG(1, 2))
		tick := time.NewTicker(33 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				for i := 0; i < 40; i++ {
					f.Fill(rng.IntN(600), rng.IntN(380), 10+rng.IntN(60), 8+rng.IntN(20), byte(rng.IntN(256)), byte(rng.IntN(256)), byte(rng.IntN(256)))
				}
			}
		}
	}()

	type delivery struct {
		seq   uint64
		bytes int
		sent  time.Time
	}
	queue := make(chan delivery, 30)
	linkDone := make(chan struct{})
	var acks sync.WaitGroup
	defer func() { close(queue); <-linkDone; acks.Wait() }()
	var mu sync.Mutex
	var delivered []delivery // what arrived, with arrival in sent
	var worst time.Duration
	go func() { // the link: one frame at a time, at its rate
		defer close(linkDone)
		free := time.Now()
		for d := range queue {
			start := d.sent.Add(oneWay)
			if free.After(start) {
				start = free
			}
			done := start.Add(time.Duration(float64(d.bytes*8) / link * float64(time.Second)))
			free = done
			time.Sleep(time.Until(done))
			mu.Lock()
			worst = max(worst, done.Sub(d.sent)-oneWay)
			delivered = append(delivered, delivery{seq: d.seq, bytes: d.bytes, sent: done})
			var recent int
			var first time.Time
			for _, x := range delivered {
				if done.Sub(x.sent) <= time.Second {
					if first.IsZero() {
						first = x.sent
						continue
					}
					recent += x.bytes
				}
			}
			mu.Unlock()
			recv := 0.0
			if done.After(first) {
				recv = float64(recent*8) / done.Sub(first).Seconds() / 1000
			}
			v.Received(d.seq, float64(done.UnixNano())/1e6, recv)
			seq := d.seq
			acks.Add(1)
			time.AfterFunc(oneWay, func() { defer acks.Done(); v.Ack(seq) })
		}
	}()

	deadline := time.After(8 * time.Second)
	measureFrom := time.Now().Add(4 * time.Second)
	for {
		select {
		case msg := <-v.Messages():
			if msg.Type != "frame" {
				continue
			}
			queue <- delivery{seq: msg.Seq, bytes: len(msg.Data) * 3 / 4, sent: time.Now()}
		case <-deadline:
			mu.Lock()
			defer mu.Unlock()
			var bytes, frames int
			for _, d := range delivered {
				if d.sent.After(measureFrom) {
					bytes += d.bytes
					frames++
				}
			}
			secs := time.Since(measureFrom).Seconds()
			v.mu.Lock()
			kbps, fps := v.rate.kbps, v.rate.fps()
			v.mu.Unlock()
			t.Logf("settled: cap %.0f kbps, %d fps allowed; delivered %.0f kbps at %.1f fps; worst queue %v",
				kbps, fps, float64(bytes*8)/1000/secs, float64(frames)/secs, worst.Round(time.Millisecond))
			if kbps > 1100 {
				t.Errorf("cap %.0f kbps on a 1000 kbit/s link", kbps)
			}
			if got := float64(bytes*8) / secs; got > link*1.05 {
				t.Errorf("delivered %.0f bit/s over a %.0f bit/s link", got, link)
			}
			if got := float64(bytes*8) / secs; got < link*0.5 {
				t.Errorf("delivered only %.0f bit/s over a %.0f bit/s link", got, link)
			}
			if worst > time.Second {
				t.Errorf("frames queued up to %v", worst)
			}
			return
		}
	}
}
