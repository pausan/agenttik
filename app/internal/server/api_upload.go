package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Files dropped on the Tree from the desktop arrive here, from the browser,
// in pieces. The bytes come over HTTP rather than as a path to copy because
// the window may be showing another machine's project: routeRemote forwards
// these requests like any other, so one endpoint copies across machines too.
// See specs/095-tree-file-drop.md.

// uploadChunkBytes is the most one request may carry. It stays under the
// BodyLimit with room to spare, which matters twice on a remote: once here,
// once on the machine it is forwarded to.
const uploadChunkBytes = 2 << 20

var uploadToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

// uploadEntry writes one piece of a dropped file. The pieces go to a hidden
// file beside the target, appended in order, and the last one renames it into
// place: a copy that fails or is cancelled halfway never leaves half a file
// under the real name, nor destroys the file it was going to replace.
//
// With dir=1 it makes a folder instead, and an existing one is fine: a dropped
// folder's empty subfolders are made this way.
func (s *Server) uploadEntry(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	rel, abs, err := resolveEntry(root, c.Query("path"))
	if err != nil {
		return err
	}
	if c.QueryBool("dir") {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return badRequest("cannot create %s: %v", rel, err)
		}
		return c.JSON(entryRef{Path: rel, Dir: true})
	}
	part, err := uploadPart(abs, c.Query("upload"))
	if err != nil {
		return err
	}
	offset, err1 := strconv.ParseInt(c.Query("offset"), 10, 64)
	size, err2 := strconv.ParseInt(c.Query("size"), 10, 64)
	data := c.Body()
	end := offset + int64(len(data))
	if err1 != nil || err2 != nil || offset < 0 || end > size || len(data) > uploadChunkBytes {
		return badRequest("invalid upload range")
	}
	replace := c.QueryBool("replace")
	// Checked on every piece, so a file that appears mid-copy is not
	// replaced unasked either.
	if info, err := os.Lstat(abs); err == nil && (!replace || info.IsDir()) {
		return badRequest("%s already exists", rel)
	}

	var f *os.File
	if offset == 0 {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return badRequest("cannot create %s: %v", rel, err)
		}
		// Truncate rather than refuse: a first piece sent again after a
		// dropped connection starts the same file over.
		f, err = os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	} else {
		f, err = os.OpenFile(part, os.O_WRONLY|os.O_APPEND, 0)
	}
	if err != nil {
		return badRequest("cannot write %s: %v", rel, err)
	}
	// A piece out of order would leave a hole or a repeat in the middle of
	// the file, so the file's length has to be where this piece begins.
	if info, err := f.Stat(); err != nil || info.Size() != offset {
		f.Close()
		return fiber.NewError(fiber.StatusConflict, "upload of "+rel+" is out of order")
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return badRequest("cannot write %s: %v", rel, err)
	}
	if end < size {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err := os.Rename(part, abs); err != nil {
		os.Remove(part)
		return badRequest("cannot write %s: %v", rel, err)
	}
	return c.JSON(entryRef{Path: rel})
}

// cancelUpload removes what a cancelled or failed copy left behind.
func (s *Server) cancelUpload(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	_, abs, err := resolveEntry(root, c.Query("path"))
	if err != nil {
		return err
	}
	part, err := uploadPart(abs, c.Query("upload"))
	if err != nil {
		return err
	}
	if err := os.Remove(part); err != nil && !os.IsNotExist(err) {
		return badRequest("cannot remove the partial copy: %v", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// uploadPart names the hidden file a copy is written to. The client picks the
// token, so the name is checked to be nothing but one.
func uploadPart(abs, token string) (string, error) {
	if !uploadToken.MatchString(token) {
		return "", badRequest("invalid upload token")
	}
	return filepath.Join(filepath.Dir(abs), ".agenttik-"+token+".part"), nil
}
