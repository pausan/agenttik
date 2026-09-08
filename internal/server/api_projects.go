package server

import (
	"os"
	"path/filepath"
	"strconv"

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

func (s *Server) deleteProject(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return badRequest("invalid project id")
	}
	if err := s.store.DeleteProject(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func expandHome(path string) string {
	if path == "~" || len(path) > 1 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
