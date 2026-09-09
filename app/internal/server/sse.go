package server

import (
	"bufio"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/pausan/agenttik/app/internal/runner"
)

// heartbeat keeps proxies and idle connections from timing the stream out.
const heartbeat = 25 * time.Second

// streamAll serves everything the UI is watching over one connection:
// `?sessions=<id>,<id>&projects=<id>`. Sessions carry their own live events
// and a project carries the end of every turn run in it, from any number of
// sessions at once.
//
// The topics travel in the query rather than one connection per tab because a
// browser holds only a handful of connections to one origin, and a dozen open
// conversations would use them all up and stall the API. The UI reopens this
// stream when a tab opens or closes.
func (s *Server) streamAll(c *fiber.Ctx) error {
	var topics []string
	for _, id := range splitList(c.Query("sessions")) {
		topics = append(topics, id)
	}
	for _, id := range splitList(c.Query("projects")) {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return badRequest("invalid project id %q", id)
		}
		topics = append(topics, runner.ProjectTopic(n))
	}
	// Unknown ids are not an error: a session deleted in another window would
	// otherwise break the whole stream. They simply never fire.
	return s.stream(c, topics)
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// stream serves a set of hub topics as SSE until the client leaves or the app
// quits.
func (s *Server) stream(c *fiber.Ctx, topics []string) error {
	events, unsubscribe := s.runner.Hub().SubscribeMany(topics)

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
