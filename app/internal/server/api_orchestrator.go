package server

import (
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
)

func (s *Server) getOrchestratorConfig(c *fiber.Ctx) error {
	cfg, err := s.store.GetOrchestratorConfig()
	if err != nil {
		return err
	}
	return c.JSON(cfg)
}

func (s *Server) putOrchestratorConfig(c *fiber.Ctx) error {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil || body.Enabled == nil {
		return badRequest("enabled is required")
	}
	cfg, err := s.store.GetOrchestratorConfig()
	if err != nil {
		return err
	}
	if *body.Enabled {
		// No prompt files are written: the project prompt is a preference,
		// leaving this workspace empty on its first enable.
		if err := os.MkdirAll(cfg.Path, 0o755); err != nil {
			return badRequest("create orchestrator folder: %v", err)
		}
	} else if cfg.ProjectID != 0 {
		if err := s.canArchiveProject(cfg.ProjectID); err != nil {
			return err
		}
	}
	if err := s.store.SetOrchestratorEnabled(*body.Enabled); err != nil {
		return err
	}
	s.projectsChanged()
	return s.getOrchestratorConfig(c)
}

func (s *Server) resetOrchestratorPrompt(c *fiber.Ctx) error {
	if err := s.store.ResetOrchestratorPrompt(); err != nil {
		return err
	}
	s.projectsChanged()
	return s.getOrchestratorConfig(c)
}

func (s *Server) canArchiveProject(id int64) error {
	busy, err := s.store.HasProjectWork(id)
	if err != nil {
		return err
	}
	if busy {
		return badRequest("stop the project's active or queued tasks before archiving it")
	}
	return nil
}

// Commands from the orchestrator use the same routes as the UI. Every open
// window must hear their changes, including one with no project selected.
func (s *Server) projectsChanged() {
	s.runner.Hub().Publish(runner.ProjectsTopic, runner.Event{
		Event: agent.Event{Type: runner.EventProjectsChanged},
	})
}
