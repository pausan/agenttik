package server

import (
	"bufio"
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
	// maxTreeEntries bounds one tree listing: files, ignored files and empty
	// folders together. Projects stay well inside it — the largest one here
	// holds 159,768, nearly all of them ignored dependencies. It is for the
	// folder that is not a project: a home directory is 9.6 million files, a
	// 1 GB answer the server built in memory and the window never finished
	// parsing.
	maxTreeEntries = 200_000
	maxFileBytes   = 2 << 20 // 2 MiB
	gitTimeout     = 5 * time.Second
)

// skipDirs are excluded from repository discovery and filesystem watching.
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

// A path the client gives absolute names a file it found outside the project:
// a transcript link into another checkout, a log under /tmp. Reading one is
// allowed. Writing is not — saving, and the three entry edits, keep taking
// the path relative and so keep the project boundary.
func resolveReadableFile(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return resolveInRoot(root, path)
}

// previewFilePath resolves what the preview and its metadata read, and says
// which root to read it against. A working copy may sit outside the project,
// so root is then only the project the tab was opened from. A revision cannot:
// git answers for the files of one repository, which is the root to find it in.
func (s *Server) previewFilePath(c *fiber.Ctx) (root, rel, abs string, err error) {
	if path := c.Query("path"); filepath.IsAbs(path) && c.Query("rev") == "" {
		if root, err = s.projectRoot(c); err != nil {
			return "", "", "", err
		}
		abs, err = resolveReadableFile(root, path)
		return root, path, abs, err
	}
	if root, err = s.repositoryRoot(c); err != nil {
		return "", "", "", err
	}
	if rel, err = s.repositoryFilePath(c, root); err != nil {
		return "", "", "", err
	}
	abs, err = resolveInRoot(root, rel)
	return root, rel, abs, err
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
//
// Truncated says the folder holds more than one listing carries, and this is
// only the first part of it. See listTree.
type projectFiles struct {
	Files     []string `json:"files"`
	Ignored   []string `json:"ignored"`
	Dirs      []string `json:"dirs"`
	Truncated bool     `json:"truncated,omitempty"`
}

func (s *Server) projectTree(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	return c.JSON(listTree(root, maxTreeEntries))
}

// listTree fills one listing of at most max entries. The project's own files
// come first, the ignored ones get what is left and the empty folders the
// rest: a dependency folder can hold a hundred times the project, and must
// never push a real file out. A cut listing names no empty folders, since it
// cannot tell a folder holding nothing from one whose files fell past the cut.
func listTree(root string, max int) projectFiles {
	files, ignored, cut := listFiles(root, max)
	tree := projectFiles{Files: files, Ignored: ignored, Dirs: []string{}, Truncated: cut}
	if !cut {
		tree.Dirs, tree.Truncated = emptyDirs(root, files, ignored, max-len(files)-len(ignored))
	}
	return tree
}

// listFiles prefers git, which gives us .gitignore handling for free and is
// far faster than walking a large working tree. Outside git, it walks every
// folder except repository metadata. Either way it stops at max entries and
// says whether it had to.
func listFiles(root string, max int) ([]string, []string, bool) {
	if isGitRepo(root) {
		if files, ignored, cut, err := listRepository(root, max); err == nil {
			return files, ignored, cut
		}
	}
	files, cut := walkFiles(root, max)
	sort.Strings(files)
	return files, []string{}, cut
}

// listRepository lists what git tracks, then what it would track, then what
// it ignores, each from what the one before left of max. Asked for the first
// two at once, git prints the untracked files first, so a folder not yet in
// .gitignore would push the project out of its own listing.
func listRepository(root string, max int) ([]string, []string, bool, error) {
	tracked, cut, err := gitList(root, max, "ls-files", "--cached")
	if err != nil {
		return nil, nil, false, err
	}
	files := withoutDeleted(root, tracked)
	if cut {
		return files, []string{}, true, nil
	}
	untracked, cut, err := gitList(root, max-len(files), "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, nil, false, err
	}
	files = append(files, untracked...)
	if cut {
		return files, []string{}, true, nil
	}
	ignored, cut := listIgnored(root, max-len(files))
	return files, ignored, cut, nil
}

// listIgnored lists the files git ignore rules cover, at most max of them.
func listIgnored(root string, max int) ([]string, bool) {
	// --ignored only means anything alongside --exclude-standard: without it
	// git has no set of patterns to call something ignored against.
	ignored, cut, err := gitList(root, max, "ls-files", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return []string{}, false
	}
	return ignored, cut
}

// walkFiles lists a folder that is not a repository a level at a time, so a
// folder too large to list whole keeps its upper levels: a home directory
// shows its own folders rather than the first files of ~/.cache. It stops at
// max files and says whether it had to.
func walkFiles(root string, max int) ([]string, bool) {
	paths := []string{}
	queue := []string{""}
	for len(queue) > 0 {
		rel := queue[0]
		queue = queue[1:]
		// An unreadable folder is skipped, not fatal; whatever it gave up
		// before failing is still listed.
		entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		for _, e := range entries {
			path := e.Name()
			if rel != "" {
				path = rel + "/" + path
			}
			// A symlink is not an IsDir, so one to a folder is listed rather
			// than followed out of the project.
			if e.IsDir() {
				if e.Name() != ".git" {
					queue = append(queue, path)
				}
				continue
			}
			if len(paths) == max {
				return paths, true
			}
			paths = append(paths, path)
		}
	}
	return paths, false
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
//
// It names at most max folders and says whether there were more.
func emptyDirs(root string, files, ignored []string, max int) ([]string, bool) {
	out := []string{}
	cut := false
	files, ignored = sorted(files), sorted(ignored)

	var walk func(dir, rel string)
	walk = func(dir, rel string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // unreadable is simply a folder we cannot report on
		}
		for _, e := range entries {
			if cut {
				return
			}
			// A symlink is not an IsDir, which is deliberate: following one
			// would leave the project and could loop.
			if !e.IsDir() || e.Name() == ".git" {
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
				if len(out) == max {
					cut = true
					return
				}
				out = append(out, child)
			}
			walk(filepath.Join(dir, e.Name()), child)
		}
	}
	walk(root, "")
	return out, cut
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
	Path     string `json:"path"`
	Status   string `json:"status"`
	Staged   bool   `json:"staged"`
	Unstaged bool   `json:"unstaged"`
	Original string `json:"-"`
}

func (s *Server) projectChanges(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return c.JSON([]changedFile{})
	}
	out, err := runGit(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return err
	}
	files := parseStatus(out)
	prefix := s.repositoryPrefix(c, root)
	for i := range files {
		files[i].Path = prefix + files[i].Path
	}
	return c.JSON(files)
}

// NUL delimiters preserve filenames containing whitespace, quotes and arrows.
func parseStatus(out string) []changedFile {
	files := []changedFile{}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		line := entries[i]
		if len(line) < 4 {
			continue
		}
		f := changedFile{Path: line[3:], Status: strings.TrimSpace(line[:2]),
			Staged: line[0] != ' ' && line[0] != '?', Unstaged: line[1] != ' '}
		if strings.ContainsAny(line[:2], "RC") && i+1 < len(entries) {
			i++
			f.Original = entries[i]
		}
		files = append(files, f)
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
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	rel, err := s.repositoryFilePath(c, root)
	if err != nil {
		return err
	}
	if _, err := resolveInRoot(root, rel); err != nil {
		return err
	}
	if !isGitRepo(root) {
		return c.JSON(fileDiff{Path: rel})
	}

	context := diffContext(c)
	var out string
	if _, err := runGit(root, "ls-files", "--error-unmatch", "--", rel); err == nil {
		// HEAD covers staged and unstaged together; a repository with no
		// commits has no HEAD, and there the index is the only baseline.
		if out, err = gitDiff(root, "diff", "--no-color", context, "HEAD", "--", rel); err != nil {
			out, _ = gitDiff(root, "diff", "--no-color", context, "--cached", "--", rel)
		}
	} else {
		out, _ = gitDiff(root, "diff", "--no-color", context, "--no-index", "--", os.DevNull, rel)
	}

	body := fileDiff{Path: rel, Partial: len(out) > maxFileBytes}
	if body.Partial {
		out = out[:maxFileBytes]
	}
	body.Diff = out
	return c.JSON(body)
}

// Whole-file context stays subject to the response and rendering limits.
func diffContext(c *fiber.Ctx) string {
	if c.Query("context") == "full" {
		return "--unified=2147483647"
	}
	return "--unified=3"
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
	abs, err := resolveReadableFile(root, rel)
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
	return runGitWithTimeout(dir, gitTimeout, args...)
}

func runGitWithTimeout(dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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

// gitList runs a git listing and keeps its first max lines. Git is stopped
// once there is one more than that, so a repository holding millions of
// untracked or ignored files costs what the cap does rather than what git
// would have printed. Running out of time cuts a listing the same way.
func gitList(dir string, max int, args ...string) ([]string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	lines := []string{}
	scan := bufio.NewScanner(stdout)
	for scan.Scan() {
		if len(lines) == max {
			// Git is still writing the rest, and dies of being stopped:
			// that exit status says nothing about the lines already read.
			cancel()
			_ = cmd.Wait()
			return lines, true, nil
		}
		lines = append(lines, scan.Text())
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return lines, true, nil
		}
		return nil, false, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return lines, false, nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// maxRawBytes bounds a preview. Files are read whole into memory to be sent,
// so this is the largest one the pane will show rather than a limit on what
// the project may contain.
const maxRawBytes = 16 << 20 // 16 MiB

// rawTypes is every image and font the raw endpoint serves, and the content type each
// is served as: an allowlist rather than a sniff, because these bytes come
// back on the app's own origin. SVG is deliberately absent — it is markup and
// can carry scripts, so its preview renders the text of the open tab instead
// of a URL that could also be opened on its own.
var rawTypes = map[string]string{
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".bmp":   "image/bmp",
	".ico":   "image/x-icon",
	".apng":  "image/apng",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

// rawRev bounds what may reach git as a revision: a hex hash or HEAD, either
// optionally with a ^ for its parent, which is the baseline side of a commit's
// image diff.
var rawRev = regexp.MustCompile(`^(HEAD|[0-9a-f]{4,40})\^?$`)

// projectRawImage serves image and font bytes, for the views that render a file
// rather than read it: the preview, and the two sides of an image diff. rev
// names a revision to read it from instead of the working tree.
func (s *Server) projectRawImage(c *fiber.Ctx) error {
	root, rel, abs, err := s.previewFilePath(c)
	if err != nil {
		return err
	}
	kind, ok := rawTypes[strings.ToLower(filepath.Ext(rel))]
	if !ok {
		return badRequest("%s is not an image or font agenttik renders", rel)
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
