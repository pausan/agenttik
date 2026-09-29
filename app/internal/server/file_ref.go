package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Missing links search only on demand. Stat candidates first, then walk once
// per root without following directory symlinks or reading file contents.
func locateFileRef(root, missing, target string, hints []string) string {
	name := filepath.Base(missing)
	suffix := filepath.Clean(target)
	if target == "" || filepath.IsAbs(suffix) {
		suffix = name
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
