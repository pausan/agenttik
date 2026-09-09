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
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err,
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
