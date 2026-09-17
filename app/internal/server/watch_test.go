package server

import (
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
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
