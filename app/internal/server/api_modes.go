package server

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

// Modes are the library of standing prompts projects and tasks open their
// conversations with. See specs/101-modes.md.

// maxPromptChars bounds a standing prompt — a mode's text or a project's own.
// Both open every conversation linked to them, where their whole length is
// paid for in context. The orchestrator prompt the app ships with, the longest
// it has, is under 7,000 characters, so the limit leaves room for several of
// those and still catches a pasted document. The fields warn from 8,000.
const maxPromptChars = 32_000

func checkPromptLength(what, text string) error {
	if n := utf8.RuneCountInString(text); n > maxPromptChars {
		return badRequest("%s is too long: %d characters, the limit is %d", what, n, maxPromptChars)
	}
	return nil
}

func (s *Server) listModes(c *fiber.Ctx) error {
	modes, err := s.store.ListModes()
	if err != nil {
		return err
	}
	return c.JSON(modes)
}

func (s *Server) createMode(c *fiber.Ctx) error {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.Body) == "" {
		return badRequest("title and body are required")
	}
	if err := checkPromptLength("mode text", strings.TrimSpace(body.Body)); err != nil {
		return err
	}
	m, err := s.store.CreateMode(body.Title, body.Body)
	if err != nil {
		return err
	}
	s.modesChanged()
	return c.Status(fiber.StatusCreated).JSON(m)
}

// updateMode takes either field; the one left out keeps its value.
func (s *Server) updateMode(c *fiber.Ctx) error {
	id, err := modeID(c)
	if err != nil {
		return err
	}
	var body struct {
		Title *string `json:"title"`
		Body  *string `json:"body"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	m, err := s.store.GetMode(id)
	if err != nil {
		return err
	}
	if body.Title != nil {
		m.Title = strings.TrimSpace(*body.Title)
	}
	if body.Body != nil {
		m.Body = strings.TrimSpace(*body.Body)
		// Only new text is measured: a rename leaves whatever is stored.
		if err := checkPromptLength("mode text", m.Body); err != nil {
			return err
		}
	}
	if m.Title == "" || m.Body == "" {
		return badRequest("title and body cannot be empty")
	}
	if err := s.store.UpdateMode(id, m.Title, m.Body); err != nil {
		return err
	}
	s.modesChanged()
	return c.JSON(m)
}

// deleteMode also unlinks it, so projects change with it.
func (s *Server) deleteMode(c *fiber.Ctx) error {
	id, err := modeID(c)
	if err != nil {
		return err
	}
	if err := s.store.DeleteMode(id); err != nil {
		return err
	}
	s.modesChanged()
	s.projectsChanged()
	return c.SendStatus(fiber.StatusNoContent)
}

func modeID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return 0, badRequest("invalid mode id")
	}
	return id, nil
}

// checkMode accepts a mode that exists, or 0. A task or a schedule (defers)
// also accepts store.ModeNone, since its 0 defers to the project's mode; a
// project's 0 is already none.
func (s *Server) checkMode(id int64, defers bool) error {
	switch {
	case id == 0, id == store.ModeNone && defers:
		return nil
	case id > 0:
		if _, err := s.store.GetMode(id); err != nil {
			return badRequest("unknown mode %d", id)
		}
		return nil
	}
	return badRequest("invalid mode %d", id)
}

func (s *Server) modesChanged() {
	s.runner.Hub().Publish(runner.ProjectsTopic, runner.Event{
		Event: agent.Event{Type: runner.EventModesChanged},
	})
}
