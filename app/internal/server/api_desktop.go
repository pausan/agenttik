package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/store"
)

// ConfigureDesktop is called before serving. Status reports native startup errors.
func (s *Server) ConfigureDesktop(status func() string) (store.DesktopConfig, error) {
	s.desktopStatus = status
	return s.store.GetDesktopConfig()
}

func (s *Server) getDesktopConfig(c *fiber.Ctx) error {
	config, err := s.store.GetDesktopConfig()
	if err != nil {
		return err
	}
	status := ""
	if s.desktopStatus != nil {
		status = s.desktopStatus()
	}
	return c.JSON(struct {
		store.DesktopConfig
		Available bool   `json:"available"`
		Error     string `json:"error"`
	}{config, s.desktopStatus != nil, status})
}

func (s *Server) putDesktopConfig(c *fiber.Ctx) error {
	if s.desktopStatus == nil {
		return badRequest("tray settings are only available in the desktop app")
	}
	var config store.DesktopConfig
	if err := c.BodyParser(&config); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if err := store.ValidateDesktopShortcut(config.ToggleShortcut); err != nil {
		return badRequest("%v", err)
	}
	if err := s.store.SetDesktopConfig(config); err != nil {
		return err
	}
	return s.getDesktopConfig(c)
}
