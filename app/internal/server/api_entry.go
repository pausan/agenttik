package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// The Tree's own file operations: make a file or a folder, rename one, delete
// one. They work on the project folder directly rather than through git, so
// they read the same whether the project is a repository or not, and what
// they do shows up in the Changed pane like any other edit.

// entryRef is what all three answer with: the path that now exists — the new
// one, for a rename — and whether it is a folder.
type entryRef struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// createEntry makes an empty file or a folder. Missing parents are created
// with it, so a path typed into the Tree's New file box lands where it says
// instead of failing on a folder that is not there yet.
func (s *Server) createEntry(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	var body struct {
		Path string `json:"path"`
		Dir  bool   `json:"dir"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	rel, abs, err := resolveEntry(root, body.Path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err == nil {
		return badRequest("%s already exists", rel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return badRequest("cannot create %s: %v", rel, err)
	}
	if body.Dir {
		err = os.Mkdir(abs, 0o755)
	} else {
		var f *os.File
		// O_EXCL rather than a plain create: two windows on the same project
		// must not have one of them truncate what the other just wrote.
		if f, err = os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil {
			err = f.Close()
		}
	}
	if err != nil {
		return badRequest("cannot create %s: %v", rel, err)
	}
	return c.JSON(entryRef{Path: rel, Dir: body.Dir})
}

// renameEntry moves a file or a folder inside the project. The new path may
// carry folders of its own — "sub/name.go" moves as it renames — which is why
// it is resolved like any other path rather than checked for being a bare
// name.
func (s *Server) renameEntry(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	var body struct {
		Path string `json:"path"`
		To   string `json:"to"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	fromRel, fromAbs, err := resolveEntry(root, body.Path)
	if err != nil {
		return err
	}
	toRel, toAbs, err := resolveEntry(root, body.To)
	if err != nil {
		return err
	}
	from, err := os.Lstat(fromAbs)
	if err != nil {
		return badRequest("cannot rename %s: %v", fromRel, err)
	}
	// os.SameFile rather than a path comparison: on a filesystem that ignores
	// case, renaming README to ReadMe finds itself at the destination, and
	// that is a rename to allow rather than a collision to refuse.
	if to, err := os.Lstat(toAbs); err == nil && !os.SameFile(from, to) {
		return badRequest("%s already exists", toRel)
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return badRequest("cannot rename %s: %v", fromRel, err)
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return badRequest("cannot rename %s: %v", fromRel, err)
	}
	return c.JSON(entryRef{Path: toRel, Dir: from.IsDir()})
}

// deleteEntry removes a file, or a folder with everything in it. It is
// permanent — the pane asks before calling this — so the guards that matter
// are in resolveEntry: the project folder itself and git's own directory are
// not deletable from here.
func (s *Server) deleteEntry(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel, abs, err := resolveEntry(root, c.Query("path"))
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return badRequest("cannot delete %s: %v", rel, err)
	}
	if info.IsDir() {
		err = os.RemoveAll(abs)
	} else {
		err = os.Remove(abs)
	}
	if err != nil {
		return badRequest("cannot delete %s: %v", rel, err)
	}
	return c.JSON(entryRef{Path: rel, Dir: info.IsDir()})
}

// resolveEntry maps a client path to the entry these three work on, and
// returns it both ways: relative for the answer, absolute for the syscall.
//
// It is deliberately not resolveInRoot. That one follows the path's own
// symlink, which is right for reading a file through a link and wrong here —
// the row in the tree stands for the link, so deleting it must unlink it
// rather than destroy what it points at. The nearest existing parent is
// resolved instead, which is also what an entry that does not exist yet
// leaves to check: a folder linked out of the project cannot be written
// through either way.
func resolveEntry(root, rel string) (string, string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", "", badRequest("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", "", badRequest("path must be relative to the project")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." {
		return "", "", badRequest("%s is the project folder itself", rel)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", badRequest("path escapes the project folder")
	}
	slashed := filepath.ToSlash(clean)
	if first, _, _ := strings.Cut(slashed, "/"); first == ".git" {
		return "", "", badRequest("git's own folder cannot be edited from the tree")
	}

	abs := filepath.Join(root, clean)
	for dir := filepath.Dir(abs); ; {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			if resolved != root && !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
				return "", "", badRequest("path escapes the project folder")
			}
			return slashed, abs, nil
		}
		if !os.IsNotExist(err) {
			return "", "", badRequest("cannot open %s: %v", rel, err)
		}
		// Creating into folders that are not there yet is allowed, so the
		// walk climbs to the deepest one that is. The project root always is.
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", badRequest("cannot open %s", rel)
		}
		dir = parent
	}
}
