package server

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) analytics(c *fiber.Ctx) error {
	days, err := strconv.Atoi(c.Query("days", "7"))
	if err != nil || days < 1 || days > 3650 {
		return badRequest("days must be an integer from 1 to 3650")
	}
	to := time.Now().UnixMilli()
	from := to - int64(days)*24*60*60*1000
	rows, err := s.store.Analytics(from, to)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"from": from, "to": to, "rows": rows})
}
