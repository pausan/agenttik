package server

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
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

// subscriptionLimits returns the signed-in CLI's own subscription allowance,
// never an estimate from token totals. There are two ways to get one and a
// provider has at most one of them:
//
//   - ask, if the CLI answers a read-only local query (Codex app-server);
//   - remember, if it only volunteers a reading mid-turn (Claude Code, on a
//     rate_limit_event line). The newest turn that reported one is served,
//     stamped with when it was recorded so the panel can say how old it is.
//
// A provider with neither has no allowance bars at all.
func (s *Server) subscriptionLimits(c *fiber.Ctx) error {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return badRequest("unknown provider %q", name)
	}
	if metered, ok := provider.(agent.Metered); ok {
		limits, err := metered.SubscriptionLimits(c.UserContext())
		if err != nil {
			return err
		}
		if limits == nil {
			limits = []agent.RateLimit{}
		}
		return c.JSON(limits)
	}

	body, at, err := s.store.LatestRateLimits(name)
	if err != nil {
		return err
	}
	limits := []agent.RateLimit{}
	if body != "" {
		if err := json.Unmarshal([]byte(body), &limits); err != nil {
			// A reading we can no longer read is not worth an error: the panel
			// simply shows no bars, as it does before the first turn.
			return c.JSON([]agent.RateLimit{})
		}
		for i := range limits {
			limits[i].ReportedAt = at
		}
	}
	return c.JSON(limits)
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
