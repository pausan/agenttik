package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/toolpaths"
)

// Missing links search only on demand. Paths the task's own tool calls named
// come first, newest first; then stat candidates, then walk once per root
// without following directory symlinks or reading file contents.
func locateFileRef(root, missing, target string, hints, touched []string) string {
	name := filepath.Base(missing)
	suffix := filepath.Clean(target)
	if target == "" || filepath.IsAbs(suffix) {
		suffix = name
	}
	if found := locateTouched(name, suffix, touched); found != "" {
		return found
	}
	roots := []string{}
	seen := map[string]bool{}
	addRoot := func(path string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			seen[path] = true
			roots = append(roots, path)
		}
	}
	if len(hints) > 100 {
		hints = hints[:100]
	}
	for _, hint := range hints {
		path, err := resolveReadableFile(root, hint)
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil {
			if !info.IsDir() {
				path = filepath.Dir(path)
			}
			addRoot(path)
		}
	}
	addRoot(filepath.Dir(missing))
	addRoot(root)
	for _, dir := range roots {
		for _, tail := range []string{suffix, name} {
			path := filepath.Join(dir, tail)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	remaining := maxTreeEntries
	visited := map[string]bool{}
	for _, dir := range roots {
		found := ""
		filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			remaining--
			if remaining <= 0 || time.Now().After(deadline) {
				return filepath.SkipAll
			}
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if visited[path] || (path != dir && skipDirs[entry.Name()]) {
					return filepath.SkipDir
				}
				visited[path] = true
				return nil
			}
			if entry.Name() == name {
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
					found = path
					return filepath.SkipAll
				}
			}
			return nil
		})
		if found != "" {
			return found
		}
		if remaining <= 0 || time.Now().After(deadline) {
			break
		}
	}
	return ""
}

// locateTouched prefers the file the task worked on: one it named whose path
// ends with the link, then the link inside a folder it worked in, then a file
// of that name there. Only stats; touched folders are never walked.
func locateTouched(name, suffix string, touched []string) string {
	if len(touched) > maxTouchedPaths {
		touched = touched[len(touched)-maxTouchedPaths:]
	}
	regular := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular()
	}
	sep := string(filepath.Separator)
	for i := len(touched) - 1; i >= 0; i-- {
		if strings.HasSuffix(touched[i], sep+suffix) && regular(touched[i]) {
			return touched[i]
		}
	}
	dirs := []string{}
	seen := map[string]bool{}
	for i := len(touched) - 1; i >= 0; i-- {
		dir := touched[i]
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = filepath.Dir(dir)
		}
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, tail := range []string{suffix, name} {
		for _, dir := range dirs {
			if path := filepath.Join(dir, tail); regular(path) {
				return path
			}
		}
	}
	return ""
}

// maxTouchedPaths bounds the stats one link click can cost.
const maxTouchedPaths = 200

// sessionPaths lists what a task's tool calls named, oldest first. The agent
// runs in the project folder, so relative paths start there. A session from
// another project names nothing here.
func (s *Server) sessionPaths(projectID int64, sessionID string) []string {
	if sessionID == "" {
		return nil
	}
	sess, err := s.store.GetSession(sessionID)
	if err != nil || sess.ProjectID != projectID {
		return nil
	}
	project, err := s.store.GetProject(projectID)
	if err != nil {
		return nil
	}
	calls, err := s.store.ToolCalls(sessionID)
	if err != nil {
		return nil
	}
	tracker := toolpaths.New(project.Path)
	turn := int64(-1)
	for _, call := range calls {
		if call.TurnID != turn {
			turn = call.TurnID
			tracker.NewTurn()
		}
		tracker.AddSummary(call.Content)
	}
	return tracker.Paths()
}
