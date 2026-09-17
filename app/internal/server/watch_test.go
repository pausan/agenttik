package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/pausan/agenttik/app/internal/runner"
)

// The refresh loop this guards against: `git status` touches .git/index's stat
// cache, which arrives as an attribute-only event. Treating that as news runs
// `git status` again, so the pair never settles.
func TestWatchFilter(t *testing.T) {
	root := filepath.Join("/repo")
	skip := func(path string) bool { return filepath.Base(path) == "node_modules" }
	interesting := watchFilter(root, skip)

	index := filepath.Join(root, ".git", "index")
	source := filepath.Join(root, "main.go")

	for _, tc := range []struct {
		name string
		ev   fsnotify.Event
		want bool
	}{
		{"index stat refresh is ignored", fsnotify.Event{Name: index, Op: fsnotify.Chmod}, false},
		{"index write still counts", fsnotify.Event{Name: index, Op: fsnotify.Write}, true},
		{"index rename still counts", fsnotify.Event{Name: index, Op: fsnotify.Create}, true},
		{"the lock git holds is ignored", fsnotify.Event{Name: filepath.Join(root, ".git", "index.lock"), Op: fsnotify.Create}, false},
		{"HEAD moving counts", fsnotify.Event{Name: filepath.Join(root, ".git", "HEAD"), Op: fsnotify.Write}, true},
		{"a mode change in the tree counts", fsnotify.Event{Name: source, Op: fsnotify.Chmod}, true},
		{"a write in the tree counts", fsnotify.Event{Name: source, Op: fsnotify.Write}, true},
		{"a skipped folder is ignored", fsnotify.Event{Name: filepath.Join(root, "node_modules"), Op: fsnotify.Write}, false},
	} {
		if got := interesting(tc.ev); got != tc.want {
			t.Errorf("%s: interesting(%v) = %v, want %v", tc.name, tc.ev, got, tc.want)
		}
	}
}

// openFDs counts this process's descriptors. /dev/fd is the kqueue-relevant
// view on macOS and a symlink to /proc/self/fd on Linux.
func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("cannot count descriptors: %v", err)
	}
	return len(entries)
}

// settle waits for want to hold, so the test follows the watcher goroutine
// rather than a sleep.
func settle(t *testing.T, want func(int) bool, what string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if n := openFDs(t); want(n) {
			return n
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s, descriptors now %d", what, openFDs(t))
	return 0
}

// A watcher is recreated whenever the last stream on a project goes and
// another arrives — a project switch, or the stream re-subscribing. On macOS
// kqueue opens a descriptor per watched *file*, so a cycle that does not give
// them back exhausts the process's allowance rather than its memory.
func TestWatcherReleasesDescriptors(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("checks macOS kqueue's per-file descriptors")
	}
	root := t.TempDir()
	const dirs, perDir = 8, 40
	for d := range dirs {
		dir := filepath.Join(root, fmt.Sprintf("dir%d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f := range perDir {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", f)), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	w := newWatchers(runner.NewHub())
	baseline := openFDs(t)

	for cycle := range 5 {
		release := w.acquire(1, root)
		t.Cleanup(release)
		// Wait for the walk to actually take descriptors, so the release
		// below is giving something back.
		settle(t, func(n int) bool { return n > baseline+dirs }, fmt.Sprintf("cycle %d to start watching", cycle))
		release()
		settle(t, func(n int) bool { return n <= baseline+dirs }, fmt.Sprintf("cycle %d to hand descriptors back", cycle))
	}

	// Under the v1.9.0 kqueue defect every cycle leaked one descriptor per
	// watched file, so five cycles would sit far above this.
	if got := openFDs(t); got > baseline+dirs {
		t.Errorf("after 5 watcher cycles: %d descriptors, baseline %d", got, baseline)
	}
}
