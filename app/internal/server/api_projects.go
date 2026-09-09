package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) listProjects(c *fiber.Ctx) error {
	projects, err := s.store.ListProjects()
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
	abs, err := filepath.Abs(expandHome(body.Path))
	if err != nil {
		return badRequest("invalid path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return badRequest("%s is not a directory", abs)
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

// updateProject renames a project; the folder is fixed for its lifetime.
func (s *Server) updateProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return badRequest("name is required")
	}
	if err := s.store.SetProjectName(id, name); err != nil {
		return err
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

func expandHome(path string) string {
	if path == "~" || len(path) > 1 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
