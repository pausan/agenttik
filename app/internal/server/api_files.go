package server

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// projectFiles is the tree listing: what git tracks or would track, and what
// it ignores, kept apart rather than mixed. The pane needs to know which is
// which — ignored files are drawn grey and their folders start shut — and the
// order matters here too. See listFiles.
//
// Dirs carries the folders holding no listed file. A tree built from file
// paths cannot imply those, and a folder just created from the Tree is
// exactly one of them. See emptyDirs.
type projectFiles struct {
	Files   []string `json:"files"`
	Ignored []string `json:"ignored"`
	Dirs    []string `json:"dirs"`
}

func (s *Server) projectTree(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	files, ignored, err := listFiles(root)
	if err != nil {
		return err
	}
	dirs := emptyDirs(root, files, ignored, maxTreeEntries-len(files)-len(ignored))
	return c.JSON(projectFiles{Files: files, Ignored: ignored, Dirs: dirs})
}

// listFiles prefers git, which gives us .gitignore handling for free and is
// far faster than walking a large working tree.
//
// The cap is spent on the project's own files first and the ignored ones get
// what is left: a dependency folder is thousands of entries — 13,789 against
// 140 in agenttik's own repository — and must never push a real file out of
// the listing. Walking by hand has no notion of ignoring, so there the
// dependency folders are skipped as they always were and nothing comes back
// grey.
func listFiles(root string) ([]string, []string, error) {
	if isGitRepo(root) {
		out, err := runGit(root, "ls-files", "--cached", "--others", "--exclude-standard")
		if err == nil {
			files := trimTo(withoutDeleted(root, splitLines(out)), maxTreeEntries)
			return files, listIgnored(root, maxTreeEntries-len(files)), nil
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
		paths = append(paths, filepath.ToSlash(rel))
		if len(paths) >= maxTreeEntries {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk project: %w", err)
	}
	sort.Strings(paths)
	return trimTo(paths, maxTreeEntries), []string{}, nil
}

// listIgnored is what .gitignore covers. It lists the files inside an ignored
// folder rather than the folder itself, which is what makes it the long half
// of the answer and why it carries a bound of its own.
func listIgnored(root string, max int) []string {
	if max <= 0 {
		return []string{}
	}
	// --ignored only means anything alongside --exclude-standard: without it
	// git has no set of patterns to call something ignored against.
	out, err := runGit(root, "ls-files", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return []string{}
	}
	return trimTo(splitLines(out), max)
}

// emptyDirs names the folders no listed file lives in. The tree is built from
// file paths, so a folder holding nothing would otherwise not appear at all —
// and a folder just created from the Tree is exactly that until something is
// written into it.
//
// The walk costs one read per folder the project actually has: it descends
// only where the listing says there is something, so a dependency folder is
// seen once at its own level and never entered. A folder whose contents are
// all ignored is already in the tree through those files and is neither named
// here nor walked, which is what keeps `node_modules` out of both.
func emptyDirs(root string, files, ignored []string, max int) []string {
	out := []string{}
	if max <= 0 {
		return out
	}
	files, ignored = sorted(files), sorted(ignored)

	var walk func(dir, rel string)
	walk = func(dir, rel string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // unreadable is simply a folder we cannot report on
		}
		for _, e := range entries {
			if len(out) >= max {
				return
			}
			// A symlink is not an IsDir, which is deliberate: following one
			// would leave the project and could loop.
			if !e.IsDir() || skipDirs[e.Name()] {
				continue
			}
			child := e.Name()
			if rel != "" {
				child = rel + "/" + e.Name()
			}
			holdsFiles := hasUnder(files, child)
			if !holdsFiles {
				if hasUnder(ignored, child) {
					continue
				}
				out = append(out, child)
			}
			walk(filepath.Join(dir, e.Name()), child)
		}
	}
	walk(root, "")
	return out
}

// hasUnder reports whether any listed path lives inside dir. The lists are in
// byte order, so this is a binary search rather than a set of every ancestor
// of every path — the ignored half alone is 13,789 of them here.
func hasUnder(list []string, dir string) bool {
	prefix := dir + "/"
	i := sort.SearchStrings(list, prefix)
	return i < len(list) && strings.HasPrefix(list[i], prefix)
}

// sorted is what hasUnder needs and what it is nearly always already given:
// git writes its listings in byte order and the hand-walked fallback sorts
// its own. A path git had to quote is the exception, and one of those must
// not quietly turn a dependency folder into an empty one.
func sorted(list []string) []string {
	if sort.StringsAreSorted(list) {
		return list
	}
	out := append([]string(nil), list...)
	sort.Strings(out)
	return out
}

// withoutDeleted drops the tracked files that are no longer on disk. Git keeps
// listing them until the deletion is staged, and a tree that still showed them
// would open tabs on files that are not there. The usual answer is empty, so
// this costs one git call and nothing else.
func withoutDeleted(root string, paths []string) []string {
	out, err := runGit(root, "ls-files", "--deleted")
	if err != nil {
		return paths
	}
	gone := splitLines(out)
	if len(gone) == 0 {
		return paths
	}
	drop := make(map[string]bool, len(gone))
	for _, path := range gone {
		drop[path] = true
	}
	kept := paths[:0]
	for _, path := range paths {
		if !drop[path] {
			kept = append(kept, path)
		}
	}
	return kept
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
	// --no-optional-locks keeps a read from writing: `git status` otherwise
	// refreshes the index on disk, which the filesystem watcher would see as
	// a change, refresh for, and see again.
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks"}, args...)...)
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

// maxRawBytes bounds a preview. Images are read whole into memory to be sent,
// so this is the largest one the pane will show rather than a limit on what
// the project may contain.
const maxRawBytes = 16 << 20 // 16 MiB

// rawTypes is every image the raw endpoint serves, and the content type each
// is served as: an allowlist rather than a sniff, because these bytes come
// back on the app's own origin. SVG is deliberately absent — it is markup and
// can carry scripts, so its preview renders the text of the open tab instead
// of a URL that could also be opened on its own.
var rawTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".apng": "image/apng",
}

// rawRev bounds what may reach git as a revision: a hex hash or HEAD, either
// optionally with a ^ for its parent, which is the baseline side of a commit's
// image diff.
var rawRev = regexp.MustCompile(`^(HEAD|[0-9a-f]{4,40})\^?$`)

// projectRawImage serves an image's bytes, for the views that render a file
// rather than read it: the preview, and the two sides of an image diff. rev
// names a revision to read it from instead of the working tree.
func (s *Server) projectRawImage(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel := c.Query("path")
	abs, err := resolveInRoot(root, rel)
	if err != nil {
		return err
	}
	kind, ok := rawTypes[strings.ToLower(filepath.Ext(rel))]
	if !ok {
		return badRequest("%s is not an image agenttik renders", rel)
	}
	data, err := rawImage(root, abs, rel, c.Query("rev"))
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, kind)
	// The file is local and an agent can rewrite it at any moment, so a copy
	// held in the browser would keep showing what is no longer there.
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Send(data)
}

// rawImage reads the working copy, or the revision asked for.
func rawImage(root, abs, rel, rev string) ([]byte, error) {
	if rev == "" {
		info, err := os.Stat(abs)
		if err != nil {
			return nil, badRequest("cannot read %s: %v", rel, err)
		}
		if info.Size() > maxRawBytes {
			return nil, badRequest("%s is larger than the %d MiB preview limit", rel, maxRawBytes>>20)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, badRequest("cannot read %s: %v", rel, err)
		}
		return data, nil
	}
	if !isGitRepo(root) {
		return nil, badRequest("%s is not a git repository", root)
	}
	if !rawRev.MatchString(rev) {
		return nil, badRequest("invalid revision")
	}
	// A file that is not in that revision is the ordinary answer for one side
	// of a diff — an image just added, or one deleted — so it is a plain not
	// found, which the pane showing it draws as an empty side.
	data, err := gitBlob(root, rev+":"+filepath.ToSlash(filepath.Clean(rel)))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("%s is not in %s", rel, rev))
	}
	return data, nil
}

// gitBlob reads one path at one revision as bytes. runGit hands back a string,
// and an image is not one.
func gitBlob(root, spec string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "show", spec)
	cmd.Dir = root
	out := &capped{max: maxRawBytes}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s: %v: %s", spec, err, strings.TrimSpace(stderr.String()))
	}
	return out.buf.Bytes(), nil
}

// capped collects a command's output up to a limit, so a blob far larger than
// anything the pane could show is refused while it streams rather than after
// it is all in memory.
type capped struct {
	buf bytes.Buffer
	max int
}

func (w *capped) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, fmt.Errorf("output is larger than %d bytes", w.max)
	}
	return w.buf.Write(p)
}
