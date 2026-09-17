package server

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
)

const (
	// fsQuiet is how long the tree must settle before a change is announced.
	// A save that writes a temp file and renames it, or a checkout that
	// touches a thousand files, is then one refresh rather than a thousand.
	fsQuiet = 250 * time.Millisecond
	// fsMaxWait bounds that settling: a build that keeps writing must still
	// reach the UI instead of being pushed back for as long as it runs.
	fsMaxWait = 2 * time.Second
	// maxWatchDirs caps the watch descriptors one project may take. A
	// descriptor per directory is cheap, but the user's inotify allowance is
	// shared with their editor and is not ours to exhaust.
	maxWatchDirs = 4096
)

// watchers keeps one filesystem watcher per project, shared by every stream
// looking at that project and stopped when the last one goes.
type watchers struct {
	hub *runner.Hub

	mu   sync.Mutex
	open map[int64]*projectWatch
}

type projectWatch struct {
	refs int
	stop chan struct{}
}

func newWatchers(hub *runner.Hub) *watchers {
	return &watchers{hub: hub, open: make(map[int64]*projectWatch)}
}

// acquire starts watching root unless something already is, and returns the
// release for this caller. Two windows on the same project share one watcher.
func (w *watchers) acquire(projectID int64, root string) func() {
	w.mu.Lock()
	pw, ok := w.open[projectID]
	if !ok {
		pw = &projectWatch{stop: make(chan struct{})}
		w.open[projectID] = pw
		go w.run(projectID, root, pw.stop)
	}
	pw.refs++
	w.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			if pw.refs--; pw.refs > 0 {
				return
			}
			// Only drop the entry if it is still ours: the project may have
			// been re-acquired under a new root in the meantime.
			if w.open[projectID] == pw {
				delete(w.open, projectID)
			}
			close(pw.stop)
		})
	}
}

// run watches the working tree and publishes one event per burst of changes.
// Nothing about what moved is reported: the panes re-read the whole listing
// anyway, and a path would only tempt them to patch it half-correctly.
func (w *watchers) run(projectID int64, root string, stop <-chan struct{}) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watch %s: %v", root, err)
		return
	}
	defer fsw.Close()

	tree := &treeWatch{fsw: fsw, skip: skipper(root)}
	tree.add(root)
	// Git's own directory is watched one level deep. Staging, committing and
	// switching branches move the index or HEAD, which changes what the
	// Changed pane says without any file in the working tree moving.
	_ = fsw.Add(filepath.Join(root, ".git"))
	interesting := watchFilter(root, tree.skip)

	// A nil channel blocks forever, which is what waiting for the first
	// change should do.
	var fire <-chan time.Time
	var timer *time.Timer
	var firstAt time.Time

	for {
		select {
		case <-stop:
			return

		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			if !interesting(ev) {
				continue
			}
			// A directory can already hold files by the time we hear it was
			// created, so it is walked rather than only added. On anything
			// that is not a directory this is one stat.
			if ev.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
				tree.add(ev.Name)
			}
			now := time.Now()
			if fire == nil {
				firstAt = now
			}
			wait := min(fsQuiet, max(0, fsMaxWait-now.Sub(firstAt)))
			if timer == nil {
				timer = time.NewTimer(wait)
			} else {
				timer.Stop()
				timer.Reset(wait)
			}
			fire = timer.C

		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watch %s: %v", root, err)

		case <-fire:
			fire = nil
			w.hub.Publish(runner.FilesTopic(projectID), runner.Event{
				ProjectID: projectID,
				Event:     agent.Event{Type: runner.EventFilesChanged},
			})
		}
	}
}

// watchFilter decides which events are worth a refresh. It is not the same
// question as which directories are worth watching: .git is never walked, but
// its own entries are exactly what tells us that a commit or a staging has
// changed what the Changed pane should say. The lock file git holds while it
// writes is left out — the index it then writes is the event that matters.
func watchFilter(root string, skip func(path string) bool) func(ev fsnotify.Event) bool {
	gitDir := filepath.Join(root, ".git")
	return func(ev fsnotify.Event) bool {
		if filepath.Dir(ev.Name) == gitDir {
			// Every `git status` refreshes the index's stat cache, which
			// arrives as an attribute-only change to .git/index. Answering
			// that with a refresh runs `git status` again, and the two never
			// settle: on macOS the pair spins several times a second forever,
			// walking the whole tree each time. Staging and committing write
			// the index rather than only touching it, so what the Changed
			// pane actually needs still gets through.
			if ev.Op == fsnotify.Chmod {
				return false
			}
			return filepath.Base(ev.Name) != "index.lock"
		}
		return !skip(ev.Name)
	}
}

// treeWatch adds directories to a watcher, up to the cap.
type treeWatch struct {
	fsw  *fsnotify.Watcher
	skip func(path string) bool
	dirs int
}

// add walks dir and watches every directory under it that the project's
// listing would show. Errors are not fatal: a folder that cannot be walked is
// simply one whose changes we will not hear about.
func (t *treeWatch) add(dir string) {
	if t.dirs >= maxWatchDirs {
		return
	}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if t.skip(path) {
			return filepath.SkipDir
		}
		if t.dirs >= maxWatchDirs {
			return fs.SkipAll
		}
		if err := t.fsw.Add(path); err != nil {
			return filepath.SkipDir
		}
		t.dirs++
		return nil
	})
}

// skipper decides what is not worth watching: what the listing leaves out
// anyway, plus everything git is told to ignore. A build folder can hold more
// files than the project and none of them ever reach the tree or the changed
// list, so watching it would only spend descriptors and wake the UI for
// nothing.
func skipper(root string) func(path string) bool {
	ignored := ignoredDirs(root)
	sep := string(os.PathSeparator)
	return func(path string) bool {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return true
		}
		for ; rel != "." && rel != sep && rel != ""; rel = filepath.Dir(rel) {
			if skipDirs[filepath.Base(rel)] || ignored[rel] {
				return true
			}
		}
		return false
	}
}

// ignoredDirs asks git for the directories it ignores. `--directory` collapses
// each one to a single entry, so this is the set of subtrees to leave alone
// rather than a list of every file in them.
func ignoredDirs(root string) map[string]bool {
	out := map[string]bool{}
	if !isGitRepo(root) {
		return out
	}
	list, err := runGit(root, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return out
	}
	for _, line := range splitLines(list) {
		if strings.HasSuffix(line, "/") {
			out[filepath.Clean(strings.Trim(line, `"`))] = true
		}
	}
	return out
}
