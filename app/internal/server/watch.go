package server

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
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
	// maxWatchDirs caps the directories one project may watch under inotify,
	// which spends one watch per directory from an allowance shared with the
	// user's editor.
	maxWatchDirs = 4096
	// maxWatchFDs caps the descriptors one project may spend under kqueue,
	// which opens one for every file in a watched directory as well as for the
	// directory. Uncapped, a large project took more than the process may
	// hold, and every socket and file opened after that failed.
	maxWatchFDs = 8192
)

// kqueue says whether fsnotify watches with kqueue here, paying per file
// rather than per directory.
var kqueue = runtime.GOOS == "darwin" || strings.HasSuffix(runtime.GOOS, "bsd")

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

	tree := newTreeWatch(fsw, skipper(root))
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

// treeWatch adds directories to a watcher until its budget is spent.
type treeWatch struct {
	fsw  *fsnotify.Watcher
	skip func(path string) bool
	// left is what may still be spent: watches under inotify, descriptors
	// under kqueue, where perFile makes each entry of a directory cost one.
	left    int
	perFile bool
}

func newTreeWatch(fsw *fsnotify.Watcher, skip func(path string) bool) *treeWatch {
	if kqueue {
		return &treeWatch{fsw: fsw, skip: skip, left: maxWatchFDs, perFile: true}
	}
	return &treeWatch{fsw: fsw, skip: skip, left: maxWatchDirs}
}

// add watches dir and every directory under it that the project's listing
// would show, nearest levels first, while the budget lasts. Going level by
// level means a large project loses its deepest folders rather than whole
// branches near the root. A directory that does not fit is left out with
// everything under it, and smaller ones after it still get their turn. Errors
// are not fatal: a folder that cannot be read is simply one whose changes we
// will not hear about.
func (t *treeWatch) add(dir string) {
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return
	}
	queue := []string{dir}
	for len(queue) > 0 && t.left > 0 {
		dir := queue[0]
		queue = queue[1:]
		if t.skip(dir) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		cost := 1
		if t.perFile {
			cost += len(entries)
		}
		if cost > t.left {
			continue
		}
		if err := t.fsw.Add(dir); err != nil {
			continue
		}
		t.left -= cost
		for _, e := range entries {
			if e.IsDir() {
				queue = append(queue, filepath.Join(dir, e.Name()))
			}
		}
	}
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
