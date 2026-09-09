package server

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
)

const (
	maxTreeEntries = 20000
	maxFileBytes   = 2 << 20 // 2 MiB
	gitTimeout     = 5 * time.Second
)

// skipDirs are never walked when the project is not a git repository.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true,
	"vendor": true, "__pycache__": true,
}

func (s *Server) projectRoot(c *fiber.Ctx) (string, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return "", badRequest("invalid project id")
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(p.Path)
	if err != nil {
		return "", badRequest("project folder unavailable: %v", err)
	}
	return root, nil
}

// resolveInRoot maps a client-supplied relative path to an absolute one and
// refuses anything that escapes the project folder.
func resolveInRoot(root, rel string) (string, error) {
	if rel == "" {
		return "", badRequest("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", badRequest("path must be relative to the project")
	}
	clean := filepath.Clean(rel)
	// Reject traversal rather than silently clamping it to the root, so the
	// caller gets a clear error instead of a puzzling "no such file".
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", badRequest("path escapes the project folder")
	}
	abs := filepath.Join(root, clean)
	// Follow symlinks before checking, so a link pointing outside is caught.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", badRequest("path escapes the project folder")
	}
	return abs, nil
}

type fileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size,omitempty"`
}

func (s *Server) projectTree(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	paths, err := listFiles(root)
	if err != nil {
		return err
	}
	return c.JSON(paths)
}

// listFiles prefers git, which gives us .gitignore handling for free and is
// far faster than walking a large working tree.
func listFiles(root string) ([]string, error) {
	if isGitRepo(root) {
		out, err := runGit(root, "ls-files", "--cached", "--others", "--exclude-standard")
		if err == nil {
			return trimTo(splitLines(out), maxTreeEntries), nil
		}
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		paths = append(paths, rel)
		if len(paths) >= maxTreeEntries {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk project: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

type changedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func (s *Server) projectChanges(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return c.JSON([]changedFile{})
	}
	out, err := runGit(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	return c.JSON(parseStatus(out))
}

// parseStatus turns `git status --porcelain=v1` output into changed files.
func parseStatus(out string) []changedFile {
	files := []changedFile{}
	for _, line := range splitLines(out) {
		if len(line) < 4 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		path := line[3:]
		// Renames are reported as "old -> new"; the new path is what matters.
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		files = append(files, changedFile{Path: strings.Trim(path, `"`), Status: status})
	}
	return files
}

type fileDiff struct {
	Path    string `json:"path"`
	Diff    string `json:"diff"`
	Partial bool   `json:"partial"`
}

// projectDiff is the working-tree diff of one file, for the Diff half of a
// file tab. An untracked file has nothing to diff against HEAD, so it is
// compared with an empty file and reads as entirely new rather than as no
// change at all.
func (s *Server) projectDiff(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel := c.Query("path")
	if _, err := resolveInRoot(root, rel); err != nil {
		return err
	}
	if !isGitRepo(root) {
		return c.JSON(fileDiff{Path: rel})
	}

	var out string
	if _, err := runGit(root, "ls-files", "--error-unmatch", "--", rel); err == nil {
		// HEAD covers staged and unstaged together; a repository with no
		// commits has no HEAD, and there the index is the only baseline.
		if out, err = gitDiff(root, "diff", "--no-color", "HEAD", "--", rel); err != nil {
			out, _ = gitDiff(root, "diff", "--no-color", "--cached", "--", rel)
		}
	} else {
		out, _ = gitDiff(root, "diff", "--no-color", "--no-index", "--", os.DevNull, rel)
	}

	body := fileDiff{Path: rel, Partial: len(out) > maxFileBytes}
	if body.Partial {
		out = out[:maxFileBytes]
	}
	body.Diff = out
	return c.JSON(body)
}

// gitDiff runs one diff. Diff exits 1 when it finds differences, which is the
// normal case here, so output that git actually produced is never an error.
func gitDiff(root string, args ...string) (string, error) {
	out, err := runGit(root, args...)
	if err != nil && out == "" {
		return "", err
	}
	return out, nil
}

type fileContent struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content string `json:"content"`
	Binary  bool   `json:"binary"`
	Partial bool   `json:"partial"`
}

func (s *Server) projectFile(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel := c.Query("path")
	abs, err := resolveInRoot(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return badRequest("cannot read %s: %v", rel, err)
	}
	if info.IsDir() {
		return badRequest("%s is a directory", rel)
	}

	f, err := os.Open(abs)
	if err != nil {
		return badRequest("cannot read %s: %v", rel, err)
	}
	defer f.Close()

	buf := make([]byte, maxFileBytes)
	n, _ := f.Read(buf)
	buf = buf[:n]

	body := fileContent{Path: rel, Size: info.Size(), Partial: info.Size() > int64(n)}
	if bytes.IndexByte(buf, 0) >= 0 || !utf8.Valid(buf) {
		body.Binary = true
	} else {
		body.Content = string(buf)
	}
	return c.JSON(body)
}

type savedFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// saveProjectFile writes an edited file back. What was never read whole is
// never written back: a truncated or binary read saved over its source would
// destroy the file, so both are refused here as well as disabled in the UI.
func (s *Server) saveProjectFile(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel := c.Query("path")
	abs, err := resolveInRoot(root, rel)
	if err != nil {
		return err
	}
	var body struct {
		Content *string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.Content == nil {
		return badRequest("content is required")
	}
	if len(*body.Content) > maxFileBytes {
		return badRequest("%s is larger than the %d MiB edit limit", rel, maxFileBytes>>20)
	}

	// A new file inherits the usual 0644; an existing one keeps its own bits.
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(abs); err == nil {
		if info.IsDir() {
			return badRequest("%s is a directory", rel)
		}
		if info.Size() > int64(maxFileBytes) {
			return badRequest("%s is too large to edit", rel)
		}
		if binary, err := looksBinary(abs); err != nil || binary {
			return badRequest("%s is not a text file", rel)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return badRequest("cannot write %s: %v", rel, err)
	}

	if err := writeAtomic(abs, []byte(*body.Content), mode); err != nil {
		return badRequest("cannot write %s: %v", rel, err)
	}
	return c.JSON(savedFile{Path: rel, Size: int64(len(*body.Content))})
}

// writeAtomic replaces a file in one step, so a failed write leaves the
// original where it was rather than half of the new contents.
func writeAtomic(abs string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(abs), "."+filepath.Base(abs)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once the rename has moved it
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, abs)
}

// looksBinary reads the same prefix the browser's null check would see.
func looksBinary(abs string) (bool, error) {
	f, err := os.Open(abs)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 8<<10)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Output is returned even on a non-zero status: git diff signals "there
	// are differences" that way, and the caller decides what that means.
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func trimTo(items []string, max int) []string {
	if len(items) > max {
		return items[:max]
	}
	if items == nil {
		return []string{}
	}
	return items
}
