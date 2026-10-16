package remote

import (
	"context"
	"net/netip"
	"net/url"
	"runtime"
	"strconv"
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
