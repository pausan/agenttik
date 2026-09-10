// Command agenttik runs agent coding sessions from one local UI.
//
// By default it opens a desktop window (Wails). With --web it only starts the
// HTTP server and prints the URL. Builds without the `desktop` tag are
// web-only and fall back automatically.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/claudecode"
	"github.com/pausan/agenttik/app/internal/agent/codex"
	"github.com/pausan/agenttik/app/internal/agent/copilot"
	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/store"
)

// errNoDesktop is returned by runDesktop in web-only builds.
var errNoDesktop = errors.New("this build has no desktop window; rebuild with -tags desktop")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agenttik:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Default()
	webOnly := flag.Bool("web", false, "serve the web UI only, no desktop window")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "address to listen on")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for agenttik.db")
	flag.Parse()

	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()

	// No turn survives a restart, so clear anything left marked running. The
	// same goes for a schedule's open run: left as it was, its schedule would
	// read as busy forever and skip every fire after it.
	if err := db.ResetRunningSessions(); err != nil {
		return err
	}
	if err := db.ResetRunningScheduleRuns(); err != nil {
		return err
	}

	github := copilot.New()
	registry := agent.NewRegistry(claudecode.New(), codex.New(), github)

	// Copilot reports its own model list, and asking costs a CLI start. Ask
	// now so the UI's first request finds the answer already cached.
	go github.Models()

	turns := runner.New(db, registry, runner.NewHub())
	srv := server.New(db, registry, turns)

	defer turns.StopAll()

	// The clock runs for as long as the app does, in either shell.
	schedules, stopSchedules := context.WithCancel(context.Background())
	defer stopSchedules()
	go turns.RunSchedules(schedules)

	if !*webOnly {
		err := runDesktop(srv)
		if !errors.Is(err, errNoDesktop) {
			return err
		}
		log.Println("desktop window unavailable, serving the web UI instead")
	}
	return serveWeb(cfg, srv, turns)
}

func serveWeb(cfg config.Config, srv *server.Server, turns *runner.Runner) error {
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	fmt.Printf("agenttik listening on http://%s\n", ln.Addr())

	errs := make(chan error, 1)
	go func() { errs <- srv.Listener(ln) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errs:
		return err
	case <-stop:
		fmt.Println("\nshutting down")
		turns.StopAll()
		return srv.Shutdown()
	}
}
