package server

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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
