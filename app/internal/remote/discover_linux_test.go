package remote

import (
	"context"
	"errors"
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

// silentPort listens with a full backlog, which drops new SYNs, so dials to it
// hang like dials to an address where nothing answers.
func silentPort(t *testing.T) string {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(fd) })
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
	return strconv.Itoa(address.(*syscall.SockaddrInet4).Port)
}

func TestScanLeavesNoDialsBehind(t *testing.T) {
	port := silentPort(t)
	for name, run := range scanRuns {
		t.Run(name, func(t *testing.T) {
			goroutines, sockets := runtime.NumGoroutine(), openFiles(t)
			networks := []netip.Prefix{netip.MustParsePrefix("127.1.0.0/24")}
			if err := run(NewScanner(networks), context.Background(), port, nil, func(*url.URL, Info) error { return nil }); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for runtime.NumGoroutine() > goroutines+2 || openFiles(t) > sockets {
				if time.Now().After(deadline) {
					t.Fatalf("after the scan: %d goroutines, %d before; %d open files, %d before", runtime.NumGoroutine(), goroutines, openFiles(t), sockets)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

// A pause while dials are pending keeps them for the next Run, which probes
// every address exactly once.
func TestScannerResumesPendingDials(t *testing.T) {
	port := silentPort(t)
	for name, run := range scanRuns {
		t.Run(name, func(t *testing.T) {
			networks := []netip.Prefix{netip.MustParsePrefix("127.1.0.0/24")}
			scanner := NewScanner(networks)
			var probed atomic.Uint64
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(100*time.Millisecond, cancel)
			if err := run(scanner, ctx, port, &probed, func(*url.URL, Info) error { return nil }); !errors.Is(err, context.Canceled) {
				t.Fatalf("pause: %v", err)
			}
			if paused := probed.Load(); paused != 0 || len(scanner.retry) != 256 {
				t.Fatalf("paused with %d probed and %d to retry", paused, len(scanner.retry))
			}
			if err := run(scanner, context.Background(), port, &probed, func(*url.URL, Info) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if got := probed.Load(); got != Hosts(networks) {
				t.Fatalf("probed %d addresses", got)
			}
		})
	}
}

func openFiles(t *testing.T) int {
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

// TestScanMeasure scans real networks, for example
// AGENTTIK_SCAN_MEASURE=192.168.1.0/24,10.77.0.0/16, and reports time and
// peak memory. AGENTTIK_SCAN_IMPL=dial measures the dial fallback. Run one
// measurement per process so peak RSS is its own.
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
	run := scanRuns["Run"]
	if os.Getenv("AGENTTIK_SCAN_IMPL") == "dial" {
		run = scanRuns["dial"]
	}
	err := run(NewScanner(networks), context.Background(), port, &probed, func(u *url.URL, info Info) error {
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
