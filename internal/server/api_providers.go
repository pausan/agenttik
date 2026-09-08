package server

import (
	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/internal/agent"
)

type providerInfo struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"display_name"`
	Models      []agent.Model `json:"models"`
	Efforts     []string      `json:"efforts"`
	Available   bool          `json:"available"`
	Reason      string        `json:"reason,omitempty"`
}

func (s *Server) listProviders(c *fiber.Ctx) error {
	out := make([]providerInfo, 0, len(s.registry.All()))
	for _, p := range s.registry.All() {
		info := providerInfo{
			Name:        p.Name(),
			DisplayName: p.DisplayName(),
			Models:      p.Models(),
			Efforts:     p.Efforts(),
			Available:   true,
		}
		if err := p.Available(); err != nil {
			info.Available, info.Reason = false, err.Error()
		}
		out = append(out, info)
	}
	return c.JSON(out)
}

func (s *Server) listStars(c *fiber.Ctx) error {
	stars, err := s.store.ListStars()
	if err != nil {
		return err
	}
	return c.JSON(stars)
}

type starBody struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}

func (s *Server) addStar(c *fiber.Ctx) error {
	var b starBody
	if err := c.BodyParser(&b); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if b.Provider == "" || b.Model == "" {
		return badRequest("provider and model are required")
	}
	if err := s.store.AddStar(b.Provider, b.Model, b.Effort); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) removeStar(c *fiber.Ctx) error {
	var b starBody
	if err := c.BodyParser(&b); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if err := s.store.RemoveStar(b.Provider, b.Model, b.Effort); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
