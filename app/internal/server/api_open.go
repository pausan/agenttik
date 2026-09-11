package server

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Handing a path to the desktop it lives on. The Tree and the sidebar both
// offer it: a project's folder, or one file or folder inside it, opened by
// whatever the machine opens that kind of thing with.

// openEntry opens the project folder, or the path given inside it, with the
// system's own handler. An empty path is the project itself, which is what a
// right click on a sidebar project row sends.
func (s *Server) openEntry(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	rel, abs := "", root
	if strings.TrimSpace(body.Path) != "" {
		if rel, abs, err = resolveEntry(root, body.Path); err != nil {
			return err
		}
	}
	// Lstat rather than Stat: what the row stands for is the link itself, and
	// a broken one is worth reporting here rather than handing to the opener.
	info, err := os.Lstat(abs)
	if err != nil {
		return badRequest("cannot open %s: %v", nameOf(rel), err)
	}
	if err := openInSystem(abs); err != nil {
		return badRequest("cannot open %s: %v", nameOf(rel), err)
	}
	return c.JSON(entryRef{Path: rel, Dir: info.IsDir()})
}

func nameOf(rel string) string {
	if rel == "" {
		return "the project folder"
	}
	return rel
}

// openInSystem hands a path to the desktop's own opener — the one a double
// click in a file manager goes through — so a folder lands in the file
// browser and a file in whatever is registered for its type. It is the same
// command per platform that Wails' BrowserOpenURL runs, done here rather than
// through the window so that a web launch, and a browser on another machine,
// reach the one desktop the projects are actually on.
func openInSystem(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The opener is a launcher: it exits as soon as the real application has
	// been asked, and nothing here waits for that application. Reaping it in
	// a goroutine keeps a zombie from sitting there for the life of the app.
	go cmd.Wait()
	return nil
}
