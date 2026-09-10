// Package single keeps one agenttik per data directory. The lock file carries
// an operating-system lock for as long as the process lives, and holds the
// address its owner serves on, so a second launch can reach the first instead
// of starting a copy over the same database.
package single

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrHeld says another process owns the lock.
var ErrHeld = errors.New("another agenttik holds the data directory")

// Lock is this process's claim on a data directory. The operating system drops
// the lock when the file closes, which a crash does too.
type Lock struct{ f *os.File }

// Acquire claims path, or returns ErrHeld if another process has it.
func Acquire(path string) (*Lock, error) { return acquire(path) }

func newLock(f *os.File) (*Lock, error) {
	// A hard kill releases the lock but leaves the address behind. Drop it
	// now, so a launch that arrives before Publish reports no address rather
	// than the dead one.
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, fmt.Errorf("clear lock file: %w", err)
	}
	return &Lock{f: f}, nil
}

// Publish records the address the owner serves on. Call it once the listener
// is up. Until then Addr reads empty, and a second launch can only say that
// the app is running.
func (l *Lock) Publish(addr string) error {
	if err := l.f.Truncate(0); err != nil {
		return fmt.Errorf("clear lock file: %w", err)
	}
	if _, err := l.f.WriteAt([]byte(addr+"\n"), 0); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return l.f.Sync()
}

// Release drops the lock, and the address with it so nothing follows a dead
// port.
func (l *Lock) Release() error {
	l.f.Truncate(0)
	return l.f.Close() // closing releases the operating-system lock
}

// Addr reads the address the owner published, empty if it has none yet.
func Addr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
