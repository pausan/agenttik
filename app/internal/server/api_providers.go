package server

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

type providerInfo struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"display_name"`
	Models      []agent.Model `json:"models"`
	Efforts     []string      `json:"efforts"`
	Available   bool          `json:"available"`
	Reason      string        `json:"reason,omitempty"`

	// Accounts are the subscriptions this provider can run under, the CLI's
	// own login first and always present. MultiAccount says whether a second
	// one can be added at all. See 050-subscription-accounts.md.
	Accounts     []accountInfo `json:"accounts"`
	MultiAccount bool          `json:"multi_account"`
}

func (s *Server) listProviders(c *fiber.Ctx) error {
	// One read for every provider's rows, rather than one per provider: they
	// are a handful of rows and this is on the app's first paint.
	accounts, err := s.store.ListAccounts()
	if err != nil {
		return err
	}
	out := make([]providerInfo, 0, len(s.registry.All()))
	for _, p := range s.registry.All() {
		_, multi := p.(agent.MultiAccount)
		info := providerInfo{
			Name:         p.Name(),
			DisplayName:  p.DisplayName(),
			Models:       p.Models(),
			Efforts:      p.Efforts(),
			Available:    true,
			Accounts:     s.providerAccounts(p, accounts),
			MultiAccount: multi,
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
//
// Both halves answer for one subscription: the account named by ?account is
// the login the CLI is asked with, and the only one whose remembered
// readings are served. Two subscriptions of a provider have two allowances,
// and showing one against the other would be worse than showing none.
func (s *Server) subscriptionLimits(c *fiber.Ctx) error {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return badRequest("unknown provider %q", name)
	}
	accountID := int64(c.QueryInt("account", int(store.SystemAccount)))
	home := ""
	if accountID != store.SystemAccount {
		account, err := s.store.GetAccount(accountID)
		if err != nil {
			return err
		}
		if account.Provider != name {
			return badRequest("subscription %q does not belong to %s", account.Alias, name)
		}
		home = account.Home
		// A subscription with no login yet has no allowance of its own, and
		// must not be shown someone else's: asked about a directory nothing
		// has signed into, the Copilot CLI answers with the account in the
		// machine's vault. No bars is the honest answer until it is signed
		// in. See 050-subscription-accounts.md.
		if multi, ok := provider.(agent.MultiAccount); ok && !multi.AccountStatus(home).SignedIn {
			return c.JSON([]agent.RateLimit{})
		}
	}
	var asked []agent.RateLimit
	var askErr error
	if metered, ok := provider.(agent.Metered); ok {
		asked, askErr = metered.SubscriptionLimits(c.UserContext(), home)
		if askErr == nil && len(asked) > 0 {
			return c.JSON(asked)
		}
	}

	remembered, err := s.rememberedLimits(name, accountID)
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
func (s *Server) rememberedLimits(name string, accountID int64) ([]agent.RateLimit, error) {
	body, at, err := s.store.LatestRateLimits(name, accountID)
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
