package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// listProjects serves the sidebar's active projects, or — with ?archived=true
// — the put-away ones Settings restores from.
func (s *Server) listProjects(c *fiber.Ctx) error {
	list := s.store.ListProjects
	if c.QueryBool("archived", false) {
		list = s.store.ArchivedProjects
	}
	projects, err := list()
	if err != nil {
		return err
	}
	return c.JSON(projects)
}

func (s *Server) createProject(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.Path == "" {
		return badRequest("path is required")
	}
	abs, err := resolveProjectPath(body.Path)
	if err != nil {
		return err
	}
	if body.Name == "" {
		body.Name = filepath.Base(abs)
	}
	p, err := s.store.CreateProject(body.Name, abs)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(p)
}

func projectID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return 0, badRequest("invalid project id")
	}
	return id, nil
}

func (s *Server) getProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return err
	}
	return c.JSON(p)
}

// reorderProjects records the order the Projects sidebar was dragged into.
func (s *Server) reorderProjects(c *fiber.Ctx) error {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if len(body.IDs) == 0 {
		return badRequest("ids are required")
	}
	if err := s.store.ReorderProjects(body.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// updateProject renames a project, repoints it at a new folder, archives or
// restores it, or any combination. Each field is applied only when sent, so a
// rename does not require the path and archiving requires neither.
func (s *Server) updateProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	var body struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Archived *bool  `json:"archived"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	name := strings.TrimSpace(body.Name)
	path := strings.TrimSpace(body.Path)
	if name == "" && path == "" && body.Archived == nil {
		return badRequest("name, path or archived is required")
	}
	if name != "" {
		if err := s.store.SetProjectName(id, name); err != nil {
			return err
		}
	}
	if path != "" {
		abs, err := resolveProjectPath(path)
		if err != nil {
			return err
		}
		if err := s.store.SetProjectPath(id, abs); err != nil {
			return err
		}
	}
	if body.Archived != nil {
		if err := s.store.SetProjectArchived(id, *body.Archived); err != nil {
			return err
		}
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return err
	}
	return c.JSON(p)
}

func (s *Server) deleteProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProject(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// projectStats totals every session ever run in the project.
func (s *Server) projectStats(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(id); err != nil {
		return err
	}
	stats, err := s.store.ProjectStats(id)
	if err != nil {
		return err
	}
	return c.JSON(stats)
}

func (s *Server) projectMetrics(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(id); err != nil {
		return err
	}
	metrics, err := s.store.ProjectDailyMetrics(id)
	if err != nil {
		return err
	}
	return c.JSON(metrics)
}

// resolveProjectPath turns a user-supplied path into the absolute, existing
// directory a project points at, shared by create and update so a moved
// folder is validated the same way a new one is.
func resolveProjectPath(path string) (string, error) {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return "", badRequest("invalid path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", badRequest("%s is not a directory", abs)
	}
	return abs, nil
}

func expandHome(path string) string {
	if path == "~" || len(path) > 1 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {

			return filepath.Join(home, path[1:])
		}
	}
	return path
}
