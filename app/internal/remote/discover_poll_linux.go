package remote

import (
	"syscall"
	"time"
)

// poller reports each registered socket once, when its connect finishes.
type poller struct {
	fd     int
	events []syscall.EpollEvent
}

func newPoller() (*poller, error) {
	fd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &poller{fd: fd, events: make([]syscall.EpollEvent, 256)}, nil
}

func (p *poller) close() { syscall.Close(p.fd) }

// add registers one-shot, so a socket handed to the HTTP check never reports
// again. Closing a socket unregisters it.
func (p *poller) add(fd int) error {
	return syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Events: syscall.EPOLLOUT | syscall.EPOLLONESHOT, Fd: int32(fd)})
}

func (p *poller) wait(timeout time.Duration, ready []int32) ([]int32, error) {
	n, err := syscall.EpollWait(p.fd, p.events, int((timeout+time.Millisecond-1)/time.Millisecond))
	if err == syscall.EINTR {
		return ready, nil
	}
	if err != nil {
		return ready, err
	}
	for _, event := range p.events[:n] {
		ready = append(ready, event.Fd)
	}
	return ready, nil
}

func newSocket(family int) (int, error) {
	return syscall.Socket(family, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, 0)
}
