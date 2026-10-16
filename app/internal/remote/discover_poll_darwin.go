package remote

import (
	"syscall"
	"time"
)

// poller reports each registered socket once, when its connect finishes.
type poller struct {
	fd     int
	events []syscall.Kevent_t
}

func newPoller() (*poller, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	return &poller{fd: fd, events: make([]syscall.Kevent_t, 256)}, nil
}

func (p *poller) close() { syscall.Close(p.fd) }

// add registers one-shot, so a socket handed to the HTTP check never reports
// again. Closing a socket unregisters it.
func (p *poller) add(fd int) error {
	var change [1]syscall.Kevent_t
	syscall.SetKevent(&change[0], fd, syscall.EVFILT_WRITE, syscall.EV_ADD|syscall.EV_ONESHOT)
	_, err := syscall.Kevent(p.fd, change[:], nil, nil)
	return err
}

func (p *poller) wait(timeout time.Duration, ready []int32) ([]int32, error) {
	ts := syscall.NsecToTimespec(int64(timeout))
	n, err := syscall.Kevent(p.fd, nil, p.events, &ts)
	if err == syscall.EINTR {
		return ready, nil
	}
	if err != nil {
		return ready, err
	}
	for _, event := range p.events[:n] {
		ready = append(ready, int32(event.Ident))
	}
	return ready, nil
}

// newSocket holds ForkLock so a concurrent exec cannot inherit the socket
// before it is marked close-on-exec.
func newSocket(family int) (int, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Socket(family, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return -1, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}
