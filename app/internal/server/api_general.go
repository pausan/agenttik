package server

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/store"
)

func (s *Server) getGeneralConfig(c *fiber.Ctx) error {
	config, err := s.store.GetGeneralConfig()
	if err != nil {
		return err
	}
	info, err := os.Stat(s.store.Path())
	if err != nil {
		return err
	}
	return c.JSON(struct {
		store.GeneralConfig
		DatabasePath string `json:"database_path"`
		DatabaseSize int64  `json:"database_size"`
	}{config, s.store.Path(), info.Size()})
}

// putGeneralConfig changes any subset of the settings: fields the body leaves
// out keep their stored values.
func (s *Server) putGeneralConfig(c *fiber.Ctx) error {
	config, err := s.store.GetGeneralConfig()
	if err != nil {
		return err
	}
	if err := c.BodyParser(&config); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if config.NewItemPosition != "top" && config.NewItemPosition != "bottom" {
		return badRequest("new_item_position must be top or bottom")
	}
	switch config.ApprovalMode {
	case store.ApprovalWait, store.ApprovalTimeout, store.ApprovalImmediate:
	default:
		return badRequest("approval_mode must be wait, timeout or immediate")
	}
	if config.ApprovalTimeout < 1 || config.ApprovalTimeout > 86400 {
		return badRequest("approval_timeout must be between 1 and 86400 seconds")
	}
	if err := s.store.SetGeneralConfig(config); err != nil {
		return err
	}
	return c.JSON(config)
}

func (s *Server) resetPreferences(c *fiber.Ctx) error {
	if err := s.store.ResetPreferences(); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
