package server

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pausan/agenttik/app/internal/update"
)

// EnableUpdates is only wired by the local desktop shell. Its private proxy
// adds the token; exposed listeners and remote windows cannot install updates.
func (s *Server) EnableUpdates(ctx context.Context) (string, error) {
	if s.profiles != nil && s.profiles.private {
		return "", nil
	}
	updater, err := update.New(s.store.Dir(), s.version)
	if err != nil {
		return "", err
	}
	s.updater, s.updateContext, s.updateToken = updater, ctx, uuid.NewString()
	go updater.Run(ctx)
	return s.updateToken, nil
}
func (s *Server) localUpdater(c *fiber.Ctx) bool {
	return s.updater != nil && s.updateToken != "" && subtle.ConstantTimeCompare([]byte(c.Get("X-Agenttik-Update-Token")), []byte(s.updateToken)) == 1
}
func (s *Server) getUpdate(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if !s.localUpdater(c) {
		return c.JSON(update.Status{})
	}
	return c.JSON(s.updater.Status())
}
func (s *Server) updateVersion(c *fiber.Ctx) (string, error) {
	if !s.localUpdater(c) {
		return "", fiber.NewError(403, "Updates are only available in the local desktop window")
	}
	if !strings.HasPrefix(c.Get("Content-Type"), "application/json") {
		return "", fiber.NewError(415, "Expected JSON")
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := c.BodyParser(&body); err != nil || body.Version == "" {
		return "", fiber.NewError(400, "Expected a release version")
	}
	return body.Version, nil
}
func (s *Server) ignoreUpdate(c *fiber.Ctx) error {
	version, err := s.updateVersion(c)
	if err != nil {
		return err
	}
	if err := s.updater.Ignore(version); err != nil {
		return fiber.NewError(409, err.Error())
	}
	return c.JSON(s.updater.Status())
}
func (s *Server) installUpdate(c *fiber.Ctx) error {
	version, err := s.updateVersion(c)
	if err != nil {
		return err
	}
	if err := s.updater.Start(s.updateContext, version); err != nil {
		return fiber.NewError(409, err.Error())
	}
	return c.Status(202).JSON(s.updater.Status())
}
