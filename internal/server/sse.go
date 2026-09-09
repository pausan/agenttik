package server

import (
	"bufio"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/pausan/agenttik/internal/runner"
)

// heartbeat keeps proxies and idle connections from timing the stream out.
const heartbeat = 25 * time.Second

func (s *Server) streamSession(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, err := s.store.GetSession(id); err != nil {
		return err
	}
	return s.stream(c, id)
}

// streamProject delivers the done event of every turn in the project.
func (s *Server) streamProject(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return badRequest("invalid project id")
	}
	if _, err := s.store.GetProject(id); err != nil {
		return err
	}
	return s.stream(c, runner.ProjectTopic(id))
}

// stream serves one hub topic as SSE until the client leaves or the app quits.
func (s *Server) stream(c *fiber.Ctx, topic string) error {
	events, unsubscribe := s.runner.Hub().Subscribe(topic)

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		// Flush a comment straight away. Nothing is sent until the first
		// flush, so without this the browser sits without an open stream
		// until either an event or the first heartbeat arrives.
		if _, err := w.WriteString(": open\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		for {
			select {
			case <-s.closing:
				// The app is quitting; let go of the connection so the
				// server can finish shutting down.
				return
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
