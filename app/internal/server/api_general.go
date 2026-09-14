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

func (s *Server) putGeneralConfig(c *fiber.Ctx) error {
	var config store.GeneralConfig
	if err := c.BodyParser(&config); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if config.NewItemPosition != "top" && config.NewItemPosition != "bottom" {
		return badRequest("new_item_position must be top or bottom")
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
