package server

import (
	"errors"
	"log"
	"net"
	"syscall"
	"time"
)

// retryAccept keeps a listener serving through accept errors that pass.
// fasthttp treats any accept error but a timeout as fatal and returns from
// Serve, which leaves the socket open with nobody accepting on it: clients
// connect and then wait forever. Running out of descriptors is the usual one,
// and it clears once whatever held them lets go, so this waits it out the way
// net/http does.
type retryAccept struct{ net.Listener }

func (l retryAccept) Accept() (net.Conn, error) {
	var delay time.Duration
	for {
		c, err := l.Listener.Accept()
		if err == nil || !(errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE)) {
			return c, err
		}
		delay = min(max(2*delay, 5*time.Millisecond), time.Second)
		log.Printf("accept: %v; retrying in %v", err, delay)
		time.Sleep(delay)
	}
}
