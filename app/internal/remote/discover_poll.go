//go:build linux || darwin

package remote

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

// Run starts every connect from the caller's goroutine and waits for them
// with one poller, so a silent address costs a socket and a few dozen bytes.
// Only connected sockets get a goroutine, for the HTTP check. Without a
// poller it falls back to runDial.
func (s *Scanner) Run(ctx context.Context, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("invalid port %q", port)
	}
	p, err := newPoller()
	if err != nil {
		return s.runDial(ctx, port, probed, found)
	}
	defer p.close()
	return s.runPoll(ctx, p, port, number, probed, found)
}

type pendingProbe struct {
	seq      uint64
	address  netip.Addr
	deadline time.Time
}

// queuedProbe orders pending probes by deadline. Every probe has the same
// timeout, so start order is deadline order. seq tells a reused descriptor
// from the probe that queued it.
type queuedProbe struct {
	fd  int32
	seq uint64
}

type checkResult struct {
	address     netip.Addr
	target      *url.URL
	info        Info
	err         error
	interrupted bool
}

func (s *Scanner) runPoll(ctx context.Context, p *poller, port string, number int, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	retry := s.retry
	s.retry = nil
	pending := make(map[int32]pendingProbe, probesInFlight)
	var queue []queuedProbe
	var seq uint64
	results := make(chan checkResult, 64)
	checking := 0
	exhausted := false
	var stopErr error
	ready := make([]int32, 0, 256)

	complete := func() {
		if probed != nil {
			probed.Add(1)
		}
	}
	handle := func(r checkResult) {
		checking--
		if r.interrupted || stopErr != nil {
			s.retry = append(s.retry, r.address)
			return
		}
		complete()
		if r.err == nil {
			if err := found(r.target, r.info); err != nil {
				stopErr = err
				cancel()
			}
		}
	}
	// check hands a connected socket to a goroutine for the HTTP check.
	check := func(fd int, address netip.Addr, deadline time.Time) {
		file := os.NewFile(uintptr(fd), "")
		conn, err := net.FileConn(file)
		file.Close()
		if err != nil {
			complete()
			return
		}
		checking++
		go func() {
			defer conn.Close()
			probeCtx, stop := context.WithDeadline(ctx, deadline)
			defer stop()
			target, info, err := checkConn(probeCtx, conn, &url.URL{Scheme: "http", Host: net.JoinHostPort(address.String(), port)})
			results <- checkResult{address, target, info, err, err != nil && probeCtx.Err() == context.Canceled}
		}()
	}
	start := func(address netip.Addr) {
		family, sockaddr := socketAddress(address, number)
		fd, err := newSocket(family)
		if err != nil {
			complete()
			return
		}
		deadline := time.Now().Add(probeTimeout)
		switch err := syscall.Connect(fd, sockaddr); {
		case err == nil:
			check(fd, address, deadline)
		case errors.Is(err, syscall.EINPROGRESS):
			if err := p.add(fd); err != nil {
				syscall.Close(fd)
				complete()
				return
			}
			seq++
			pending[int32(fd)] = pendingProbe{seq, address, deadline}
			queue = append(queue, queuedProbe{int32(fd), seq})
		default:
			syscall.Close(fd)
			complete()
		}
	}
	expire := func(now time.Time) {
		for len(queue) > 0 {
			q := queue[0]
			if probe, ok := pending[q.fd]; ok && probe.seq == q.seq {
				if probe.deadline.After(now) {
					return
				}
				delete(pending, q.fd)
				syscall.Close(int(q.fd))
				complete()
			}
			queue = queue[1:]
		}
	}

	for ctx.Err() == nil {
		expire(time.Now())
		for !exhausted && len(pending)+checking < probesInFlight {
			var address netip.Addr
			if len(retry) > 0 {
				address, retry = retry[0], retry[1:]
			} else if next, ok := s.nextAddress(); ok {
				address = next
			} else {
				exhausted = true
				break
			}
			start(address)
		}
		if len(pending) == 0 {
			if checking == 0 {
				if exhausted {
					break
				}
				continue
			}
			select {
			case r := <-results:
				handle(r)
			case <-ctx.Done():
			}
			continue
		}
		// Wake for the next deadline, and often enough to see finished checks
		// and cancellation.
		wait := min(time.Until(pending[queue[0].fd].deadline), 50*time.Millisecond)
		var err error
		if ready, err = p.wait(max(wait, 0), ready[:0]); err != nil {
			stopErr = err
			break
		}
		for _, fd := range ready {
			probe, ok := pending[fd]
			if !ok {
				continue
			}
			delete(pending, fd)
			if soErr, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_ERROR); err != nil || soErr != 0 {
				syscall.Close(int(fd))
				complete()
				continue
			}
			check(int(fd), probe.address, probe.deadline)
		}
	drain:
		for {
			select {
			case r := <-results:
				handle(r)
			default:
				break drain
			}
		}
	}
	// As in runDial, a deadline ends pending probes; only a pause keeps them.
	ended := errors.Is(ctx.Err(), context.DeadlineExceeded)
	for fd, probe := range pending {
		syscall.Close(int(fd))
		if ended {
			complete()
		} else {
			s.retry = append(s.retry, probe.address)
		}
	}
	for checking > 0 {
		handle(<-results)
	}
	s.retry = append(s.retry, retry...)
	if stopErr != nil {
		return stopErr
	}
	return ctx.Err()
}

func socketAddress(address netip.Addr, port int) (int, syscall.Sockaddr) {
	if address.Is4() {
		return syscall.AF_INET, &syscall.SockaddrInet4{Port: port, Addr: address.As4()}
	}
	return syscall.AF_INET6, &syscall.SockaddrInet6{Port: port, Addr: address.As16()}
}
