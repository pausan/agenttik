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
// never an estimate from token totals. There are two ways to get one:
//
//   - ask, if the CLI answers a read-only local query (Codex app-server,
//     Claude Code's own /usage). The reading is current, so it is unstamped.
//   - remember, from the mid-turn readings a CLI volunteers on its own
//     (Claude Code, on a rate_limit_event line). The newest turn that
//     reported one is served, stamped with when it was recorded so the panel
//     can say how old it is.
//
// A CLI that does both is asked first, and the remembered reading stands in
// whenever the ask fails or comes back empty — an unreachable CLI, or a report
// worded in a way the parser no longer recognises, then shows the last known
// bars instead of none. A provider that does neither has no allowance bars.
func (s *Server) subscriptionLimits(c *fiber.Ctx) error {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return badRequest("unknown provider %q", name)
	}
	var asked []agent.RateLimit
	var askErr error
	if metered, ok := provider.(agent.Metered); ok {
		asked, askErr = metered.SubscriptionLimits(c.UserContext())
		if askErr == nil && len(asked) > 0 {
			return c.JSON(asked)
		}
	}

	remembered, err := s.rememberedLimits(name)
	if err != nil {
		return err
	}
	if len(remembered) > 0 {
		return c.JSON(remembered)
	}
	if askErr != nil {
		return askErr // nothing to stand in for it, so the ask's error is the answer
	}
	return c.JSON([]agent.RateLimit{})
}

// rememberedLimits is the newest allowance a provider volunteered during a
// turn, stamped with when it was recorded.
func (s *Server) rememberedLimits(name string) ([]agent.RateLimit, error) {
	body, at, err := s.store.LatestRateLimits(name)
	if err != nil {
		return nil, err
	}
	limits := []agent.RateLimit{}
	if body == "" {
		return limits, nil
	}
	if err := json.Unmarshal([]byte(body), &limits); err != nil {
		// A reading we can no longer read is not worth an error: the panel
		// simply shows no bars, as it does before the first turn.
		return []agent.RateLimit{}, nil
	}
	for i := range limits {
		limits[i].ReportedAt = at
	}
	return limits, nil
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
