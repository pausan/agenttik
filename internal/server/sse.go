package server

import (
	"bufio"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// heartbeat keeps proxies and idle connections from timing the stream out.
const heartbeat = 25 * time.Second

func (s *Server) streamSession(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, err := s.store.GetSession(id); err != nil {
		return err
	}

	events, unsubscribe := s.runner.Hub().Subscribe(id)

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				if _, err := w.WriteString("data: " + string(payload) + "\n\n"); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
			}
			// A failed flush means the browser is gone.
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))
	return nil
}
