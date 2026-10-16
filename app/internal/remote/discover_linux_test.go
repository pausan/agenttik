package remote

import (
	"context"
	"net/netip"
	"net/url"
	"os"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// A listener with a full backlog drops new SYNs, so dials to it hang like
// dials to an address where nothing answers.
func TestScanLeavesNoDialsBehind(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Listen(fd, 0); err != nil {
		t.Fatal(err)
	}
	address, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(address.(*syscall.SockaddrInet4).Port)

	before := runtime.NumGoroutine()
	networks := []netip.Prefix{netip.MustParsePrefix("127.1.0.0/24")}
	if err := Scan(context.Background(), networks, port, nil, func(*url.URL, Info) error { return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+2 {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines still running after the scan, %d before", runtime.NumGoroutine(), before)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestScanMeasure scans real networks, for example
// AGENTTIK_SCAN_MEASURE=192.168.1.0/24,10.77.0.0/16, and reports time and
// peak memory. Run one measurement per process so peak RSS is its own.
func TestScanMeasure(t *testing.T) {
	raw := os.Getenv("AGENTTIK_SCAN_MEASURE")
	if raw == "" {
		t.Skip("set AGENTTIK_SCAN_MEASURE to comma-separated networks")
	}
	var networks []netip.Prefix
	for _, network := range strings.Split(raw, ",") {
		networks = append(networks, netip.MustParsePrefix(network))
	}
	port := os.Getenv("AGENTTIK_SCAN_PORT")
	if port == "" {
		port = "7717"
	}
	runtime.GC()
	baseline := procStatusKB(t, "VmRSS")
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/heap/stacks:bytes"}}
	var peakGoroutines, peakHeap, peakStacks uint64
	done := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			metrics.Read(samples)
			peakHeap = max(peakHeap, samples[0].Value.Uint64())
			peakStacks = max(peakStacks, samples[1].Value.Uint64())
			peakGoroutines = max(peakGoroutines, uint64(runtime.NumGoroutine()))
			select {
			case <-done:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
	var probed atomic.Uint64
	found := 0
	start := time.Now()
	err := Scan(context.Background(), networks, port, &probed, func(u *url.URL, info Info) error {
		found++
		t.Logf("found %s %q %s", u, info.Name, info.Version)
		return nil
	})
	elapsed := time.Since(start)
	close(done)
	<-sampled
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("networks=%s probed=%d found=%d elapsed=%s", raw, probed.Load(), found, elapsed.Round(10*time.Millisecond))
	t.Logf("peak goroutines=%d heap objects=%.1f MB stacks=%.1f MB RSS=%.1f MB (%.1f MB above baseline)",
		peakGoroutines, float64(peakHeap)/(1<<20), float64(peakStacks)/(1<<20), float64(procStatusKB(t, "VmHWM"))/1024, float64(procStatusKB(t, "VmHWM")-baseline)/1024)
}

func procStatusKB(t *testing.T, field string) uint64 {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(status), "\n") {
		if value, ok := strings.CutPrefix(line, field+":"); ok {
			kb, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return kb
		}
	}
	t.Fatalf("no %s in /proc/self/status", field)
	return 0
}
