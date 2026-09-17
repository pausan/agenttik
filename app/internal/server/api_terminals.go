package server

import (
	"bufio"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/pausan/agenttik/app/internal/terminals"
)

// Terminal tabs. A terminal is a shell on a pseudo-terminal in the project's
// folder — see app/internal/terminals and specs/073-terminals.md.
//
// Output comes back over SSE and keystrokes go out over POST rather than both
// sharing a WebSocket, because the desktop window is not served over a socket
// at all: Wails hands the UI's requests to an http.Handler, which can stream a
// response but cannot be hijacked into a two-way connection. SSE is the one
// live channel that works in the window, the browser and the exposed server
// alike, and the UI already keeps a stream open through all three.

// listTerminals is one project's open terminals, oldest first. The UI rebuilds
// its terminal tabs from this, so a reloaded window finds the shells it left
// running rather than orphaning them.
func (s *Server) listTerminals(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return badRequest("invalid project id")
	}
	return c.JSON(s.terminals.List(id))
}

type terminalSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// openTerminalTab starts a shell in the project's folder.
func (s *Server) openTerminalTab(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return badRequest("invalid project id")
	}
	// projectRoot resolves the folder and reports a project whose path has
	// gone missing as itself, which is the same answer the Tree gives.
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	var size terminalSize
	// A body is optional: a client that has not measured its pane yet gets
	// the default size and resizes once it has.
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&size); err != nil {
			return badRequest("invalid terminal size")
		}
	}
	info, err := s.terminals.Open(id, root, size.Cols, size.Rows)
	if err != nil {
		return err
	}
	return c.JSON(info)
}

// closeTerminalTab ends a shell and everything it started.
func (s *Server) closeTerminalTab(c *fiber.Ctx) error {
	if err := s.terminals.Close(c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// writeTerminal is what typing into a terminal sends. The body is the raw
// bytes, not JSON: they are keystrokes, including the control characters JSON
// would have to escape, and there is nothing else to say about them.
func (s *Server) writeTerminal(c *fiber.Ctx) error {
	if err := s.terminals.Write(c.Params("id"), c.Body()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// resizeTerminal passes a changed pane size on to the shell, which is how a
// full-screen program redraws itself to fit.
func (s *Server) resizeTerminal(c *fiber.Ctx) error {
	var size terminalSize
	if err := c.BodyParser(&size); err != nil {
		return badRequest("invalid terminal size")
	}
	if err := s.terminals.Resize(c.Params("id"), size.Cols, size.Rows); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// streamTerminals serves every open terminal's output over one connection,
// `?ids=<id>:<from>,<id>:<from>`, each frame naming the terminal it belongs
// to. One connection for all of them for the same reason streamAll carries
// every session: a browser holds only a handful to one origin.
//
// `from` is how many bytes of that terminal the client already has, so a
// stream reopened because a *different* terminal was opened does not repeat
// this one's output into a view still showing it. A client with nothing sends
// zero, or leaves the count off entirely.
//
// It is a second connection rather than part of streamAll because the two
// carry different things. A busy terminal writes faster than anything a turn
// publishes, and a watcher that falls behind is dropped; sharing a stream
// would mean a `find /` in a terminal could cost the window its session
// events too.
//
// Unknown ids are skipped rather than refused, so a tab whose shell has
// already gone does not break the stream for the others.
func (s *Server) streamTerminals(c *fiber.Ctx) error {
	frames, unsubscribe := s.terminals.SubscribeMany(parseWatches(c.Query("ids")))

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		// Nothing reaches the browser until the first flush, so an idle shell
		// would otherwise leave the stream looking unopened.
		if _, err := w.WriteString(": open\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		for {
			select {
			case <-s.closing:
				return
			case frame, ok := <-frames:
				if !ok {
					// Either this watcher fell behind or the app is quitting.
					// Ending the response is what makes the browser reconnect
					// and be handed the screen again.
					return
				}
				payload, err := json.Marshal(frame)
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
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))
	return nil
}

// parseWatches reads the stream's `ids` query. A malformed count is read as
// zero — the whole screen — rather than refused: the worst it costs is a
// redraw, and failing the stream over it would cost every terminal on it.
func parseWatches(query string) []terminals.Watch {
	parts := splitList(query)
	watches := make([]terminals.Watch, 0, len(parts))
	for _, part := range parts {
		id, count, _ := strings.Cut(part, ":")
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		from, err := strconv.ParseInt(count, 10, 64)
		if err != nil || from < 0 {
			from = 0
		}
		watches = append(watches, terminals.Watch{ID: id, From: from})
	}
	return watches
}

// Terminals is how the shell and the tests reach the manager.
func (s *Server) Terminals() *terminals.Manager { return s.terminals }
