// Package server exposes the HTTP API and the embedded UI. It binds to
// loopback and has no authentication: agenttik is a local tool.
package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/web"
)

type Server struct {
	app      *fiber.App
	store    *store.Store
	runner   *runner.Runner
	registry *agent.Registry

	// closing is closed by Shutdown to release the SSE handlers. Fiber waits
	// for every open connection, and a live stream never ends on its own, so
	// without this the process could not quit.
	closing   chan struct{}
	closeOnce sync.Once
}

func New(s *store.Store, reg *agent.Registry, r *runner.Runner) *Server {
	app := fiber.New(fiber.Config{
		AppName:               "agenttik",
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler,
	})
	app.Use(recover.New())

	srv := &Server{app: app, store: s, runner: r, registry: reg, closing: make(chan struct{})}
	srv.routes()

	// The UI is a Vite build, so a binary made without it says so rather than
	// serving a blank page.
	if web.Built() {
		app.Use("/", filesystem.New(filesystem.Config{
			Root:         http.FS(web.Assets()),
			Index:        "index.html",
			MaxAge:       0,
			NotFoundFile: "index.html",
		}))
	} else {
		app.Use("/", uiMissing)
	}
	return srv
}

// uiMissing answers when the binary carries no compiled UI.
func uiMissing(c *fiber.Ctx) error {
	c.Type("txt")
	return c.Status(fiber.StatusServiceUnavailable).SendString(
		"The web UI is not built into this binary. Run `make ui`, rebuild, and start agenttik again.\n")
}

func (s *Server) routes() {
	api := s.app.Group("/api")

	api.Get("/providers", s.listProviders)
	api.Get("/fs", s.browseDir)

	api.Get("/projects", s.listProjects)
	api.Post("/projects", s.createProject)
	api.Post("/projects/order", s.reorderProjects)
	api.Get("/projects/:id", s.getProject)
	api.Patch("/projects/:id", s.updateProject)
	api.Delete("/projects/:id", s.deleteProject)
	api.Get("/projects/:id/stats", s.projectStats)
	api.Get("/projects/:id/tree", s.projectTree)
	api.Get("/projects/:id/changes", s.projectChanges)
	api.Get("/projects/:id/file", s.projectFile)
	api.Post("/projects/:id/sessions/order", s.reorderSessions)

	api.Get("/sessions", s.listSessions)
	api.Post("/sessions", s.createSession)
	api.Get("/sessions/:id", s.getSession)
	api.Delete("/sessions/:id", s.deleteSession)
	api.Patch("/sessions/:id", s.updateSession)
	api.Post("/sessions/:id/messages", s.postMessage)
	api.Post("/sessions/:id/stop", s.stopSession)

	// One stream for every open tab. See streamAll.
	api.Get("/stream", s.streamAll)

	api.Get("/stars", s.listStars)
	api.Post("/stars", s.addStar)
	api.Delete("/stars", s.removeStar)
}

// shutdownTimeout bounds how long Shutdown waits for open connections.
const shutdownTimeout = 3 * time.Second

// Listen serves on addr until Shutdown is called.
func (s *Server) Listen(addr string) error { return s.app.Listen(addr) }

// Listener serves on an already-open listener, which desktop mode uses so it
// can learn the port before the window opens.
func (s *Server) Listener(ln net.Listener) error { return s.app.Listener(ln) }

// Shutdown releases the live streams first, then waits briefly for the
// remaining connections. The timeout is a backstop: a stuck client must never
// keep the app alive after the window is closed.
func (s *Server) Shutdown() error {
	s.closeOnce.Do(func() { close(s.closing) })
	return s.app.ShutdownWithTimeout(shutdownTimeout)
}

// Handler adapts the app to net/http, for the desktop shell's asset server.
func (s *Server) Handler() http.Handler { return adaptor.FiberApp(s.app) }

// apiError is the body returned for any failed API call.
type apiError struct {
	Error string `json:"error"`
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = fiber.StatusNotFound
	case errors.Is(err, runner.ErrBusy):
		code = fiber.StatusConflict
	case errors.Is(err, runner.ErrNotRunning):
		code = fiber.StatusConflict
	case errors.Is(err, agent.ErrNotImplemented):
		code = fiber.StatusNotImplemented
	default:
		var fe *fiber.Error
		if errors.As(err, &fe) {
			code = fe.Code
		}
	}
	return c.Status(code).JSON(apiError{Error: err.Error()})
}

func badRequest(format string, args ...any) error {
	return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(format, args...))
}
