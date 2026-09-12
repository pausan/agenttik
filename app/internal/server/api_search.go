package server

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) smartSearchStatus(c *fiber.Ctx) error {
	return c.JSON(s.search.Status())
}

func (s *Server) refreshSmartSearch(c *fiber.Ctx) error {
	return c.Status(fiber.StatusAccepted).JSON(s.search.Refresh())
}

func (s *Server) querySmartSearch(c *fiber.Ctx) error {
	var body struct {
		Query string   `json:"query"`
		IDs   []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	body.Query = strings.TrimSpace(body.Query)
	if body.Query == "" || len(body.Query) > 16384 || len(body.IDs) > 100000 {
		return badRequest("query must contain 1–16384 bytes and at most 100000 task ids")
	}
	scores, err := s.search.Search(body.Query, body.IDs)
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return c.JSON(scores)
}
