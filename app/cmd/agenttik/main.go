// Command agenttik runs agent coding sessions from one local UI.
//
// By default it opens a desktop window (Wails). With --web it only starts the
// HTTP server and prints the URL. Builds without the `desktop` tag are
// web-only and fall back automatically.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/claudecode"
	"github.com/pausan/agenttik/app/internal/agent/codex"
	"github.com/pausan/agenttik/app/internal/agent/copilot"
	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
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

	// One instance per data directory. A second one would share the SQLite
	// file and clear the first's running turns on the way up, so it hands the
	// user back to the copy already open instead of starting.
	lock, err := single.Acquire(cfg.LockPath())
	if errors.Is(err, single.ErrHeld) {
		return foreground(single.Addr(cfg.LockPath()))
	}
	if err != nil {
		return err
	}
	defer lock.Release()

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
		err := runDesktop(srv, lock, cfg.Addr)
		if !errors.Is(err, errNoDesktop) {
			return err
		}
		log.Println("desktop window unavailable, serving the web UI instead")
	}
	return serveWeb(cfg, srv, turns, lock)
}

func serveWeb(cfg config.Config, srv *server.Server, turns *runner.Runner, lock *single.Lock) error {
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	if err := lock.Publish(ln.Addr().String()); err != nil {
		return err
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

// raiseTimeout bounds the ask a second launch makes of the running instance.
// It is a loopback request to a process that is already up, so anything slower
// than this means it is wedged and there is nothing to wait for.
const raiseTimeout = 2 * time.Second

// foreground asks the instance already running to show itself, and reports
// what came of it. Launching agenttik twice is not an error: the second launch
// is a request to look at the first. addr is empty when that one has the lock
// but has not started listening yet.
func foreground(addr string) error {
	switch {
	case addr == "":
		fmt.Println("agenttik is already running")
	case raise("http://" + addr):
		fmt.Println("agenttik is already running; brought its window to the front")
	default:
		fmt.Printf("agenttik is already running on http://%s\n", addr)
	}
	return nil
}

// raise posts to the running instance and returns whether it had a window to
// show. Web mode has none, and neither does a desktop instance whose window
// has not opened yet.
func raise(url string) bool {
	client := http.Client{Timeout: raiseTimeout}
	res, err := client.Post(url+"/api/foreground", "application/json", nil)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var body struct {
		Raised bool `json:"raised"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return false
	}
	return body.Raised
}
