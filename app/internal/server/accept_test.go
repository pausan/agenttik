package server

import (
	"net"
	"os"
	"syscall"
	"testing"
)

// fakeListener hands out its errors in order, then a connection.
type fakeListener struct {
	net.Listener
	errs []error
}

func (l *fakeListener) Accept() (net.Conn, error) {
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	c, _ := net.Pipe()
	return c, nil
}

// Running out of descriptors used to stop the server for good: fasthttp
// returned from Serve and the socket stayed open with nobody accepting.
func TestRetryAcceptOutlastsDescriptorExhaustion(t *testing.T) {
	emfile := &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", syscall.EMFILE)}
	ln := retryAccept{&fakeListener{errs: []error{emfile, emfile}}}
	c, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v, want a connection after the retries", err)
	}
	c.Close()
}

func TestRetryAcceptReturnsOtherErrors(t *testing.T) {
	ln := retryAccept{&fakeListener{errs: []error{net.ErrClosed}}}
	if _, err := ln.Accept(); err != net.ErrClosed {
		t.Fatalf("Accept() error = %v, want %v", err, net.ErrClosed)
	}
}
