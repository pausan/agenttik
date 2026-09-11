package server

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// maxDirEntries caps one listing. Picking a project folder never needs more,
// and a directory with a million entries must not stall the UI.
const maxDirEntries = 2000

type dirRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// dirListing is one level of the filesystem, enough for the folder picker to
// show where it is, walk up, and walk down.
type dirListing struct {
	Path      string   `json:"path"`
	Parent    string   `json:"parent,omitempty"` // empty at the filesystem root
	Home      string   `json:"home"`
	Crumbs    []dirRef `json:"crumbs"`
	Dirs      []dirRef `json:"dirs"`
	Truncated bool     `json:"truncated,omitempty"`
}

// browseDir lists the folders inside ?path, defaulting to the home directory.
// It deliberately reaches outside any project: the caller is choosing which
// folder becomes a project. The server is loopback-only, so this exposes no
// more than the user's own shell already does.
func (s *Server) browseDir(c *fiber.Ctx) error {
	dir, err := resolveDir(c.Query("path"))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return badRequest("cannot read %s: %v", dir, err)
	}

	showHidden := c.QueryBool("hidden")
	dirs := make([]dirRef, 0, len(entries))
	truncated := false
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !isDir(dir, e) {
			continue
		}
		if len(dirs) >= maxDirEntries {
			truncated = true
			break
		}
		dirs = append(dirs, dirRef{Name: name, Path: filepath.Join(dir, name)})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	home, _ := os.UserHomeDir()
	return c.JSON(dirListing{
		Path:      dir,
		Parent:    parentOf(dir),
		Home:      home,
		Crumbs:    crumbs(dir),
		Dirs:      dirs,
		Truncated: truncated,
	})
}

// resolveDir turns a client-supplied path into an absolute directory, or an
// error the picker can show.
func resolveDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", badRequest("no home directory: %v", err)
		}
		path = home
	}
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return "", badRequest("invalid path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", badRequest("cannot open %s: %v", abs, err)
	}
	if !info.IsDir() {
		return "", badRequest("%s is not a directory", abs)
	}
	return abs, nil
}

// isDir treats a symlink to a directory as a directory, which is how folders
// under a symlinked home or workspace normally appear.
func isDir(parent string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(parent, e.Name()))
	return err == nil && info.IsDir()
}

func parentOf(dir string) string {
	parent := filepath.Dir(dir)
	if parent == dir {
		return "" // already at the root
	}
	return parent
}

// crumbs splits an absolute path into clickable segments, root first.
func crumbs(dir string) []dirRef {
	sep := string(os.PathSeparator)
	out := []dirRef{{Name: sep, Path: sep}}
	walked := sep
	for _, part := range strings.Split(strings.Trim(dir, sep), sep) {
		if part == "" {
			continue
		}
		walked = filepath.Join(walked, part)
		out = append(out, dirRef{Name: part, Path: walked})
	}
	return out
}

// makeDir creates one folder inside an existing one, for the picker's New
// folder button: a project can be started on a folder that does not exist
// yet without leaving the app for a shell.
func (s *Server) makeDir(c *fiber.Ctx) error {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	parent, err := resolveDir(body.Path)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return badRequest("name is required")
	}
	// One segment, and not a way back up: the picker is showing `parent`, so
	// a name that would land anywhere else is not what was asked for. Both
	// separators are refused, not just this platform's, since the name is
	// typed rather than picked.
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return badRequest("%s is a path, not a folder name", name)
	}
	dir := filepath.Join(parent, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return badRequest("cannot create %s: %v", dir, err)
	}
	return c.JSON(dirRef{Name: name, Path: dir})
}

// cloneTimeout bounds one clone. It is far longer than the read-only git
// calls the panes make: this one talks to a remote, over a repository that
// may be large.
const cloneTimeout = 10 * time.Minute

// remoteURL is what may be cloned: a scheme from the allowlist, or git's
// scp-like `user@host:path`, with an optional `user@` or `token@` in front of
// the host either way.
//
// It is an allowlist because git's transports include `ext::`, which runs a
// shell command of the URL's choosing. agenttik has no login and its server
// can be put on the network (043), so this field must not also be a way to
// run commands on the machine. The host is held to starting with a letter,
// digit or underscore for the same reason one step down: a host beginning
// with `-` is an option to whatever `ssh` this git calls.
var remoteURL = regexp.MustCompile(
	`^(?:(?:https?|ssh|git)://(?:[^/@\s]+@)?[\w][\w.-]*(?::\d+)?/\S+` +
		`|[\w][\w.-]*@[\w][\w.-]*:\S+)$`)

// cloneRepo clones a repository into a folder the user has picked, and
// answers with the folder it landed in. It is what the Add project dialog's
// git-repos mode calls per repository, before the project itself is created
// on the folder holding them all.
func (s *Server) cloneRepo(c *fiber.Ctx) error {
	var body struct {
		Path string `json:"path"`
		URL  string `json:"url"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	base, err := resolveDir(body.Path)
	if err != nil {
		return err
	}
	url := strings.TrimSpace(body.URL)
	if !remoteURL.MatchString(url) {
		return badRequest("%s is not an https, ssh or git remote", url)
	}
	name := repoName(url)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return badRequest("cannot tell which folder %s would clone into", url)
	}
	dir := filepath.Join(base, name)
	if _, err := os.Lstat(dir); err == nil {
		return badRequest("%s already exists", dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--", url, dir)
	cmd.Dir = base
	// A repository that wants credentials would otherwise sit waiting on a
	// terminal this process does not have, until the timeout. Failing at once
	// lets the dialog say so while the user is still looking at it.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A clone killed partway leaves its folder behind, and the name has
		// to be free for the retry. Nothing was there before — that was
		// checked above — so there is nothing of the user's to lose.
		os.RemoveAll(dir)
		// The URL is not repeated: the dialog shows the row this answers, and
		// git's own last line is what the row has no other way to know.
		return badRequest("cannot clone: %s", gitFailure(stderr.String()))
	}
	return c.JSON(dirRef{Name: name, Path: dir})
}

// repoName is the folder a URL clones into: git's own rule, the last path
// segment with any trailing `.git` and slash removed.
func repoName(url string) string {
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if i := strings.LastIndexAny(url, `/\:`); i >= 0 {
		url = url[i+1:]
	}
	return url
}

// gitFailure picks the line of git's complaint worth showing: its last one,
// since what comes before it is "Cloning into …".
func gitFailure(stderr string) string {
	lines := splitLines(strings.TrimSpace(stderr))
	if len(lines) == 0 {
		return "git failed"
	}
	return lines[len(lines)-1]
}
