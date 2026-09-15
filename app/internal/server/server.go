// Package server exposes the HTTP API and the embedded UI. It binds to
// loopback and asks nothing of whoever reaches it there — agenttik is a local
// tool and the window's own connection is that loopback one. The desktop
// shell can expose the same server further, on a host and port of the user's
// choosing, through app/internal/netserver; that listener alone can be put
// behind a password and an authenticator code, which is what
// app/internal/netauth is and what the routes under /api/server/auth set up.
package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/netserver"
	"github.com/pausan/agenttik/app/internal/remote"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/smartsearch"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/web"
)

type Server struct {
	profiles *profileManager
	app      *fiber.App
	version  string
	store    *store.Store
	runner   *runner.Runner
	registry *agent.Registry
	watchers *watchers
	search   *smartsearch.Service

	// closing is closed by Shutdown to release the SSE handlers. Fiber waits
	// for every open connection, and a live stream never ends on its own, so
	// without this the process could not quit.
	closing   chan struct{}
	closeOnce sync.Once

	// foreground raises the app's window, and says whether there was one to
	// raise. Set by the desktop shell before serving starts; nil in web mode.
	foreground    func() bool
	desktopStatus func() string

	// network is the desktop shell's optional exposed-server manager, and the
	// host and port it falls back to when nothing has been saved yet. Set by
	// the desktop shell before serving starts; nil in web mode, where the
	// process is already the server on whatever address it was started with.
	network            *netserver.Manager
	networkDefaultHost string
	networkDefaultPort int

	// auth is the lock that listener can be put behind. It exists in every
	// mode so the routes that set it up have something to call; only the
	// exposed listener is ever actually wrapped in it.
	//
	// creds is what it checks against, held here rather than read from
	// SQLite per request: the gate sees every request that listener takes,
	// including each frame of an open stream. The two routes that write
	// credentials are the only things that invalidate it, so a change is
	// still in force for the very next request.
	auth  *netauth.Gate
	creds atomic.Pointer[netauth.Credentials]
}

func New(s *store.Store, reg *agent.Registry, r *runner.Runner) *Server {
	app := fiber.New(fiber.Config{
		AppName:               "agenttik",
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler,
		// Without this, Query and Params hand back strings that point into
		// fasthttp's request buffer, which is recycled for the next request.
		// Session ids and stream topics outlive their handler — they end up
		// as map keys in the runner and the hub — so an aliased string turns
		// into a different one later and the entry can never be found again.
		// One copy per accessor is nothing next to the work a turn does.
		Immutable: true,
	})
	app.Use(recover.New())
	// The UI is over a megabyte of script and stylesheet, and the task list
	// behind the sidebar is a few hundred kilobytes of JSON. Over loopback
	// that costs nothing either way, but Settings › Server can put this same
	// UI in front of a browser on another machine ([043]), and there it is
	// the difference between a window that opens at once and one that spends
	// a second downloading itself. Best speed rather than best size: the
	// saving is already most of the bytes, and the rest is not worth spending
	// a local CPU on.
	//
	// The stream is left alone. It is a connection that stays open and is
	// read a frame at a time; compressing it would hold those frames in a
	// buffer waiting for more, which is the one thing it cannot do.
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
		Next:  func(c *fiber.Ctx) bool { return isStreamPath(c.Path()) },
	}))

	srv := &Server{app: app, store: s, runner: r, registry: reg,
		watchers: newWatchers(r.Hub()), closing: make(chan struct{})}
	srv.auth = netauth.New(srv.credentials)
	srv.search = smartsearch.New(filepath.Join(s.Dir(), "smart-search"), func() ([]store.Session, error) {
		return s.ListSessions(store.SessionFilter{})
	})
	app.Use(srv.routeProfile)
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

func isStreamPath(path string) bool {
	return strings.HasPrefix(path, "/api/stream")
}

// uiMissing answers when the binary carries no compiled UI.
func uiMissing(c *fiber.Ctx) error {
	c.Type("txt")
	return c.Status(fiber.StatusServiceUnavailable).SendString(
		"The web UI is not built into this binary. Run `make ui`, rebuild, and start agenttik again.\n")
}

// SetVersion supplies the build version before the server starts listening.
func (s *Server) SetVersion(version string) { s.version = version }

func (s *Server) routes() {
	s.app.Get(remote.VersionPath, func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		version := s.version
		if version == "" {
			version = "dev"
		}
		return c.JSON(remote.Info{Application: "agenttik", Version: version})
	})
	s.app.Post(remote.ConnectPath, adaptor.HTTPHandler(remote.ConnectHandler(func(u *url.URL) string { return u.String() + "/" })))
	api := s.app.Group("/api")
	api.Get("/profiles", s.listProfiles)
	api.Post("/profiles", s.createProfile)
	api.Delete("/profiles/:id", s.deleteProfile)
	api.Get("/smart-search", s.smartSearchStatus)
	api.Post("/smart-search/index", s.refreshSmartSearch)
	api.Post("/smart-search/query", s.querySmartSearch)
	api.Post("/attachments", s.uploadAttachment)
	api.Get("/attachments/:name", s.getAttachment)

	api.Get("/providers", s.listProviders)
	api.Get("/providers/:provider/subscription-limits", s.subscriptionLimits)
	api.Post("/providers/:provider/accounts", s.createAccount)
	api.Put("/providers/:provider/account", s.setDefaultAccount)
	api.Patch("/providers/:provider/accounts/:account", s.updateAccount)
	api.Delete("/providers/:provider/accounts/:account", s.deleteAccount)
	api.Get("/providers/:provider/accounts/:account/usage", s.accountUsage)
	api.Post("/providers/:provider/accounts/:account/login", s.loginAccount)
	api.Put("/providers/:provider/accounts/:account/connection", s.configureAccount)
	api.Get("/fs", s.browseDir)
	api.Post("/fs/dir", s.makeDir)
	api.Post("/fs/clone", s.cloneRepo)
	api.Post("/fs/remote", s.checkRemote)
	api.Post("/foreground", s.raiseWindow)
	api.Get("/desktop", s.getDesktopConfig)
	api.Put("/desktop", s.putDesktopConfig)
	api.Get("/action-models", s.getActionModels)
	api.Put("/action-models/:action", s.putActionModel)
	api.Get("/general", s.getGeneralConfig)
	api.Put("/general", s.putGeneralConfig)
	api.Post("/general/reset", s.resetPreferences)
	api.Get("/server", s.getServerConfig)
	api.Put("/server", s.putServerConfig)
	api.Put("/server/auth", s.putServerAuth)
	api.Post("/server/auth/totp", s.resetServerTOTP)
	api.Get("/server/auth/totp.png", s.serverTOTPQR)
	api.Get("/server/auth/code", s.serverTOTPCode)
	api.Get("/orchestrator", s.getOrchestratorConfig)
	api.Put("/orchestrator", s.putOrchestratorConfig)
	api.Post("/orchestrator/prompt/reset", s.resetOrchestratorPrompt)

	api.Get("/projects", s.listProjects)
	api.Post("/projects", s.createProject)
	api.Post("/projects/order", s.reorderProjects)
	api.Get("/projects/:id", s.getProject)
	api.Patch("/projects/:id", s.updateProject)
	api.Delete("/projects/:id", s.deleteProject)
	api.Get("/projects/:id/stats", s.projectStats)
	api.Get("/projects/:id/metrics", s.projectMetrics)
	api.Get("/projects/:id/tree", s.projectTree)
	api.Get("/projects/:id/repositories", s.projectRepositories)
	api.Get("/projects/:id/changes", s.projectChanges)
	api.Post("/projects/:id/stage", s.projectStage)
	api.Post("/projects/:id/unstage", s.projectUnstage)
	api.Post("/projects/:id/revert", s.projectRevert)
	api.Post("/projects/:id/commit", s.createCommit)
	api.Post("/projects/:id/commit-message", s.generateCommitMessage)
	api.Get("/projects/:id/log", s.projectLog)
	api.Post("/projects/:id/branches/:action", s.branchAction)
	api.Get("/projects/:id/commit", s.projectCommit)
	api.Get("/projects/:id/commit/diff", s.projectCommitDiff)
	api.Get("/projects/:id/file", s.projectFile)
	api.Get("/projects/:id/file-info", s.projectFileInfo)
	api.Put("/projects/:id/file", s.saveProjectFile)
	api.Post("/projects/:id/entry", s.createEntry)
	api.Post("/projects/:id/entry/rename", s.renameEntry)
	api.Delete("/projects/:id/entry", s.deleteEntry)
	api.Post("/projects/:id/open", s.openEntry)
	api.Get("/projects/:id/diff", s.projectDiff)
	api.Get("/projects/:id/raw", s.projectRawImage)
	api.Post("/projects/:id/sessions/order", s.reorderSessions)
	api.Post("/projects/:id/schedules/order", s.reorderSchedules)

	api.Get("/sessions", s.listSessions)
	api.Post("/sessions", s.createSession)
	api.Get("/sessions/:id", s.getSession)
	api.Delete("/sessions/:id", s.deleteSession)
	api.Patch("/sessions/:id", s.updateSession)
	api.Post("/sessions/:id/messages", s.postMessage)
	api.Post("/sessions/:id/messages/:message/edit", s.editMessage)
	api.Post("/sessions/:id/queue", s.enqueueMessage)
	api.Patch("/sessions/:id/queue/:queuedID", s.updateQueuedMessage)
	api.Post("/sessions/:id/queue/force", s.forceQueuedMessage)
	api.Post("/sessions/:id/stop", s.stopSession)

	api.Get("/schedules", s.listSchedules)
	api.Post("/schedules", s.createSchedule)
	api.Get("/schedules/:id", s.getSchedule)
	api.Patch("/schedules/:id", s.updateSchedule)
	api.Delete("/schedules/:id", s.deleteSchedule)
	api.Post("/schedules/:id/run", s.runScheduleNow)

	// One stream for every open tab. See streamAll.
	api.Get("/stream", s.streamAll)

	api.Get("/stars", s.listStars)
	api.Post("/stars", s.addStar)
	api.Delete("/stars", s.removeStar)
	api.Put("/stars/order", s.reorderStars)
	api.Get("/model-visibility", s.listModelVisibility)
	api.Put("/model-visibility", s.setModelVisibility)
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
	s.CloseProfiles()
	s.closeOnce.Do(func() { close(s.closing); s.search.Close() })
	return s.app.ShutdownWithTimeout(shutdownTimeout)
}

// Handler adapts the app to net/http, for the desktop shell's asset server.
func (s *Server) Handler() http.Handler { return adaptor.FiberApp(s.app) }

// OnForeground registers how to raise the app's window. Call it during wiring,
// before serving starts: nothing guards the field afterwards.
func (s *Server) OnForeground(raise func() bool) { s.foreground = raise }

// SetNetworkManager wires in the desktop shell's optional exposed-server
// manager, and the host and port to report and to bind when no explicit
// choice has ever been saved. Call during wiring, before serving starts, like
// OnForeground; leave unset in web mode, where GET /api/server answers that
// there is nothing to configure.
func (s *Server) SetNetworkManager(m *netserver.Manager, defaultHost string, defaultPort int) {
	m.Use(s.auth.Wrap)
	s.network = m
	s.networkDefaultHost = defaultHost
	s.networkDefaultPort = defaultPort
}

// ApplyStoredNetworkConfig starts the exposed listener if the saved setting
// says to. Call once, after SetNetworkManager and before the window opens. A
// failure — the saved port taken by something else since the last run — is
// not fatal: ApplyStoredNetworkConfig only leaves it unbound, and Status
// carries the reason to GET /api/server rather than stopping the app from
// opening at all.
func (s *Server) ApplyStoredNetworkConfig() {
	if s.network == nil {
		return
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil || !cfg.Enabled {
		return
	}
	s.network.Start(net.JoinHostPort(s.withDefaults(cfg)))
}

// credentials is what the gate on the exposed listener checks a login
// against. It is read on every attempt rather than cached, so a password set
// in Settings is in force for the very next request; a read that fails
// answers "enabled with nothing to check", which netauth treats as closed.
func (s *Server) credentials() netauth.Credentials {
	if c := s.creds.Load(); c != nil {
		return *c
	}
	return s.reloadCredentials()
}

// reloadCredentials re-reads the lock from SQLite and caches it. Two
// goroutines racing here both do the same read and store the same answer, so
// the only cost of the race is the second read. A failed read answers
// "enabled with nothing to check", which netauth closes rather than opens,
// and is not cached — the next request tries again.
func (s *Server) reloadCredentials() netauth.Credentials {
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		log.Printf("server: read auth config: %v", err)
		return netauth.Credentials{Enabled: true}
	}
	c := netauth.Credentials{
		Enabled:      cfg.AuthEnabled,
		PasswordHash: cfg.PasswordHash,
		Secret:       cfg.TOTPSecret,
	}
	s.creds.Store(&c)
	return c
}

// withDefaults fills in the app's compiled-in host and port wherever cfg
// holds the zero value, i.e. nothing has ever been saved for it.
func (s *Server) withDefaults(cfg store.ServerConfig) (host, port string) {
	host = cfg.Host
	if host == "" {
		host = s.networkDefaultHost
	}
	port = strconv.Itoa(cfg.Port)
	if cfg.Port == 0 {
		port = strconv.Itoa(s.networkDefaultPort)
	}
	return host, port
}

// raiseWindow answers the request a second launch makes instead of starting
// its own copy of the app. See app/cmd/agenttik and app/internal/single.
func (s *Server) raiseWindow(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"raised": s.foreground != nil && s.foreground()})
}

// apiError is the body returned for any failed API call.
type apiError struct {
	Error string `json:"error"`
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = fiber.StatusNotFound
	case errors.Is(err, store.ErrPathInUse):
		code = fiber.StatusConflict
	case errors.Is(err, runner.ErrBusy):
		code = fiber.StatusConflict
	case errors.Is(err, runner.ErrNotRunning):
		code = fiber.StatusConflict
	case errors.Is(err, runner.ErrForcePending):
		code = fiber.StatusConflict
	case errors.Is(err, runner.ErrUnknownAccount), errors.Is(err, runner.ErrAccountSignedOut):
		// Not a server fault: the task names a subscription that has been
		// removed, or one nothing has signed into yet. Signing in, or picking
		// another subscription for the task, is the fix.
		code = fiber.StatusBadRequest
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
