package server

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/store"
)

// listProjects serves the sidebar's visible projects, or — with
// ?archived=true / ?hidden=true — one of the two put-away lists.
func (s *Server) listProjects(c *fiber.Ctx) error {
	if c.QueryBool("hidden", false) {
		projects, err := s.store.HiddenProjects()
		if err != nil {
			return err
		}
		return c.JSON(projects)
	}
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
	// The store says the folder is taken; from here it can only be taken by
	// this same add, so say that rather than point at "another project".
	if errors.Is(err, store.ErrPathInUse) {
		return fiber.NewError(fiber.StatusConflict, "you cannot add the same folder twice")
	}
	if err != nil {
		return err
	}
	// Any open window redraws its sidebar. The one that sent this request
	// refreshes anyway; the ones that did not — another browser tab, or the
	// window behind a terminal running `agenttik --init` — only hear it here.
	s.projectsChanged()
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
	s.projectsChanged()
	return c.SendStatus(fiber.StatusNoContent)
}

// updateProject renames a project, repoints it at a new folder, sets the
// prompt its conversations open with, hides or restores it, archives or
// restores it, or any combination. Each field is applied only when sent, so a
// rename does not require the path and visibility changes require neither.
//
// The prompt is a pointer because an empty one means something: it is how the
// injection is turned off, which a blank name or path never is.
func (s *Server) updateProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	var body struct {
		Name     string  `json:"name"`
		Path     string  `json:"path"`
		Prompt   *string `json:"prompt"`
		Archived *bool   `json:"archived"`
		Hidden   *bool   `json:"hidden"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	name := strings.TrimSpace(body.Name)
	path := strings.TrimSpace(body.Path)
	if name == "" && path == "" && body.Prompt == nil && body.Archived == nil && body.Hidden == nil {
		return badRequest("name, path, prompt, archived or hidden is required")
	}
	if body.Archived != nil && *body.Archived {
		if err := s.canArchiveProject(id); err != nil {
			return err
		}
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
	if body.Prompt != nil {
		if err := s.store.SetProjectPrompt(id, strings.TrimSpace(*body.Prompt)); err != nil {
			return err
		}
	}
	if body.Archived != nil {
		if err := s.store.SetProjectArchived(id, *body.Archived); err != nil {
			return err
		}
	}
	if body.Hidden != nil {
		if err := s.store.SetProjectHidden(id, *body.Hidden); err != nil {
			return err
		}
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return err
	}
	s.projectsChanged()
	return c.JSON(p)
}

func (s *Server) deleteProject(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(id); err != nil {
		return err
	}
	// Deleting a project follows task deletion: stop its processes and clear
	// queues before removing their rows, so no work is left running unseen.
	sessions, err := s.store.ListSessions(store.SessionFilter{ProjectID: id})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		s.runner.Stop(session.ID)
	}
	if err := s.store.DeleteProject(id); err != nil {
		return err
	}
	s.projectsChanged()
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
