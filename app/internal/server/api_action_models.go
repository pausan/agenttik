package server

import (
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

var automaticActions = []struct{ ID, Label, Description string }{
	{"merge", "Merge and rebase conflicts", "Astra High or Opus High resolves conflicts. Git completes clean operations without a model."},
	{"commit_message", "Commit messages", "A lightweight model drafts a message from staged changes."},
}

func (s *Server) defaultActionModel(action string) store.ActionModel {
	// Prefer the explicit high-quality defaults; other subscriptions can offer
	// these models too. Commit messages use each provider's small-model policy.
	providers := slices.Clone(s.registry.All())
	priority := func(p agent.Provider) int {
		switch p.Name() {
		case "codex":
			return 0
		case "claude":
			return 1
		default:
			return 2
		}
	}
	slices.SortStableFunc(providers, func(a, b agent.Provider) int { return priority(a) - priority(b) })
	for _, p := range providers {
		if p.Available() != nil {
			continue
		}
		var model, effort string
		if action == "commit_message" {
			if small, ok := p.(agent.SmallModel); ok {
				model, effort = small.SmallModel()
			}
		} else {
			for _, m := range p.Models() {
				if m.ID == "gpt-6-astra" || strings.Contains(m.ID, "opus") {
					model, effort = m.ID, "high"
					break
				}
			}
		}
		if model == "" {
			continue
		}
		account, err := s.store.DefaultAccount(p.Name())
		if err != nil {
			continue
		}
		return store.ActionModel{Provider: p.Name(), AccountID: account, Model: model, Effort: effort}
	}
	return store.ActionModel{}
}

func (s *Server) actionModel(action string) (store.ActionModel, error) {
	choices, err := s.store.ActionModels()
	if err != nil {
		return store.ActionModel{}, err
	}
	choice, ok := choices[action]
	if !ok {
		choice = s.defaultActionModel(action)
	}
	if choice.Provider == "" {
		return choice, badRequest("choose a model for this action in Settings → Models → Automatic actions")
	}
	if _, err := s.chooseAccount(choice.Provider, &choice.AccountID, 0, true); err != nil {
		return choice, err
	}
	return choice, nil
}

func (s *Server) getActionModels(c *fiber.Ctx) error {
	saved, err := s.store.ActionModels()
	if err != nil {
		return err
	}
	rows := []fiber.Map{}
	for _, action := range automaticActions {
		choice, custom := saved[action.ID]
		if !custom {
			choice = s.defaultActionModel(action.ID)
		}
		rows = append(rows, fiber.Map{"id": action.ID, "label": action.Label, "description": action.Description, "choice": choice, "custom": custom})
	}
	return c.JSON(rows)
}

func (s *Server) putActionModel(c *fiber.Ctx) error {
	action := c.Params("action")
	if !slices.ContainsFunc(automaticActions, func(a struct{ ID, Label, Description string }) bool { return a.ID == action }) {
		return badRequest("unknown automatic action")
	}
	var choice store.ActionModel
	if err := c.BodyParser(&choice); err != nil {
		return badRequest("invalid model choice")
	}
	var saved *store.ActionModel
	if choice.Provider != "" {
		p, ok := s.registry.Get(choice.Provider)
		if !ok {
			return badRequest("unknown provider")
		}
		models := p.Models()
		i := slices.IndexFunc(models, func(m agent.Model) bool { return m.ID == choice.Model })
		if i < 0 {
			return badRequest("unknown model")
		}
		efforts := models[i].Efforts
		if len(efforts) == 0 {
			efforts = p.Efforts()
		}
		if choice.Effort != "" && !slices.Contains(efforts, choice.Effort) {
			return badRequest("unsupported effort")
		}
		if _, err := s.chooseAccount(choice.Provider, &choice.AccountID, 0, true); err != nil {
			return err
		}
		saved = &choice
	}
	if err := s.store.SetActionModel(action, saved); err != nil {
		return err
	}
	return s.getActionModels(c)
}
