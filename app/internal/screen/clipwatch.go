//go:build darwin || windows

package screen

import (
	"sync"
	"time"
)

// clipWatch tells subscribers when another program takes the clipboard, on
// systems that only keep a change counter: it reads the counter twice a
// second while anyone listens.
type clipWatch struct {
	seq  func() int64 // the clipboard's change counter
	read func() (string, error)

	mu   sync.Mutex
	subs map[int]chan string
	next int
	own  int64 // the counter just after this process set the clipboard
	stop chan struct{}
}

const clipPoll = 500 * time.Millisecond

func (c *clipWatch) subscribe() (<-chan string, func()) {
	ch := make(chan string, 1)
	c.mu.Lock()
	if c.subs == nil {
		c.subs = map[int]chan string{}
	}
	id := c.next
	c.next++
	c.subs[id] = ch
	if c.stop == nil {
		c.stop = make(chan struct{})
		go c.poll(c.stop)
	}
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.subs, id)
		if len(c.subs) == 0 && c.stop != nil {
			close(c.stop)
			c.stop = nil
		}
		c.mu.Unlock()
	}
}

// setOwn records the counter after this process changed the clipboard, so
// the change is not announced as another program's.
func (c *clipWatch) setOwn(n int64) {
	c.mu.Lock()
	c.own = n
	c.mu.Unlock()
}

func (c *clipWatch) poll(stop chan struct{}) {
	t := time.NewTicker(clipPoll)
	defer t.Stop()
	last := c.seq()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		n := c.seq()
		if n == last {
			continue
		}
		last = n
		c.mu.Lock()
		own := c.own == n
		c.mu.Unlock()
		if own {
			continue
		}
		text, err := c.read()
		if err != nil {
			continue
		}
		c.mu.Lock()
		for _, ch := range c.subs {
			select { // only the latest text matters
			case <-ch:
			default:
			}
			ch <- text
		}
		c.mu.Unlock()
	}
}

func (c *clipWatch) close() {
	c.mu.Lock()
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.subs = nil
	c.mu.Unlock()
}
