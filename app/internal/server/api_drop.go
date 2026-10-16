package server

import (
	"crypto/subtle"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Files dropped on the Linux desktop window. WebKitGTK gives the page no
// File objects for them, only a drag it may accept, so Wails reports the
// paths natively. The desktop shell hands them here, and the page reads them
// back through these local-only routes. It then sends them on through the
// same upload as any other drop, which also reaches remote machines.
// See specs/095-tree-file-drop.md.

type desktopDrop struct {
	mu    sync.Mutex
	token string
	paths []string
}

// EnableFileDrops returns the token the desktop window's proxy sends: only
// that window may read what was dropped on it.
func (s *Server) EnableFileDrops() string {
	s.drop.mu.Lock()
	defer s.drop.mu.Unlock()
	s.drop.token = uuid.NewString()
	return s.drop.token
}

// DropFiles replaces what the window last had dropped on it.
func (s *Server) DropFiles(paths []string) {
	s.drop.mu.Lock()
	defer s.drop.mu.Unlock()
	s.drop.paths = append([]string(nil), paths...)
}

// droppedRoots answers the dropped paths, for the desktop window only.
func (s *Server) droppedRoots(c *fiber.Ctx) ([]string, error) {
	s.drop.mu.Lock()
	defer s.drop.mu.Unlock()
	if s.drop.token == "" || subtle.ConstantTimeCompare([]byte(c.Get("X-Agenttik-Window-Token")), []byte(s.drop.token)) != 1 {
		return nil, fiber.NewError(fiber.StatusForbidden, "dropped files are only available in the desktop window")
	}
	return s.drop.paths, nil
}

type droppedFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// listDrop walks the dropped paths into the shape upload.js reads a browser
// drop into: files and empty folders, relative to where they were dropped.
func (s *Server) listDrop(c *fiber.Ctx) error {
	roots, err := s.droppedRoots(c)
	if err != nil {
		return err
	}
	files, dirs := []droppedFile{}, []string{}
	for _, root := range roots {
		base := filepath.Dir(root)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(base, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if entries, err := os.ReadDir(path); err == nil && len(entries) == 0 {
					dirs = append(dirs, rel)
				}
				return nil
			}
			// Links are followed to what they name; anything else that is
			// not a file (a socket, a device) has nothing to copy.
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			files = append(files, droppedFile{Path: rel, Size: info.Size()})
			return nil
		})
		if err != nil {
			return badRequest("cannot read %s: %v", root, err)
		}
	}
	return c.JSON(fiber.Map{"files": files, "dirs": dirs})
}

// readDrop answers one piece of a dropped file, named as listDrop named it.
func (s *Server) readDrop(c *fiber.Ctx) error {
	roots, err := s.droppedRoots(c)
	if err != nil {
		return err
	}
	// The first part names the dropped entry; the rest must stay inside it.
	first, rest, _ := strings.Cut(c.Query("path"), "/")
	inside := filepath.Clean(filepath.FromSlash(rest))
	if first == "" || filepath.IsAbs(inside) || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return badRequest("invalid path")
	}
	offset, err1 := strconv.ParseInt(c.Query("offset"), 10, 64)
	length, err2 := strconv.Atoi(c.Query("length"))
	if err1 != nil || err2 != nil || offset < 0 || length < 0 || length > uploadChunkBytes {
		return badRequest("invalid range")
	}
	for _, root := range roots {
		if filepath.Base(root) != first {
			continue
		}
		f, err := os.Open(filepath.Join(root, inside))
		if err != nil {
			return badRequest("cannot read %s: %v", c.Query("path"), err)
		}
		defer f.Close()
		buf := make([]byte, length)
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return badRequest("cannot read %s: %v", c.Query("path"), err)
		}
		c.Set("Content-Type", "application/octet-stream")
		return c.Send(buf[:n])
	}
	return fiber.ErrNotFound
}
