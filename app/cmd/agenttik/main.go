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
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
	"github.com/pausan/agenttik/app/internal/store"
)

// errNoDesktop is returned by runDesktop in web-only builds.
var errNoDesktop = errors.New("this build has no desktop window; rebuild with -tags desktop")

// version is the release this binary was built from: the MAJOR.MINOR.PATCH of
// the vMAJOR.MINOR.PATCH tag that ran the build, or the short commit when no
// such tag did. The build sets it with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agenttik:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Default()
	webOnly := flag.Bool("web", false, "serve the web UI only, no desktop window")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "`host:port` to listen on")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "`directory` holding agenttik.db")
	showHelp := flag.Bool("help", false, "show this help and exit")
	showVersion := flag.Bool("version", false, "show the version and exit")
	flag.Usage = usage
	flag.Parse()

	// Asked for, so it goes to stdout and succeeds. A bad flag gets the same
	// text on stderr from flag itself, which then exits 2.
	if *showHelp {
		flag.CommandLine.SetOutput(os.Stdout)
		usage()
		return nil
	}
	if *showVersion {
		fmt.Println("agenttik", version)
		return nil
	}

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
	providers := []agent.Provider{claudecode.New(), codex.New(), github}
	// Registered only for the end-to-end suite, which sets this; a normal
	// launch never does, so it never appears in a release. See
	// app/internal/agent/fake.
	if fake.Enabled() {
		providers = append(providers, fake.New())
	}
	registry := agent.NewRegistry(providers...)

	// Copilot reports its own model list, and asking costs a CLI start. Ask
	// now so the UI's first request finds the answer already cached.
	go github.Models()

	turns := runner.New(db, registry, runner.NewHub())
	srv := server.New(db, registry, turns)

	defer turns.StopAll()

	// The clocks run for as long as the app does, in either shell: one fires
	// due schedules, the other retries prompts whose provider was away.
	clocks, stopClocks := context.WithCancel(context.Background())
	defer stopClocks()
	go turns.RunSchedules(clocks)
	go turns.RunQueueRetries(clocks)

	if !*webOnly {
		err := runDesktop(srv, lock, cfg.Addr)
		if !errors.Is(err, errNoDesktop) {
			return err
		}
		log.Println("desktop window unavailable, serving the web UI instead")
	}
	return serveWeb(cfg, srv, turns, lock)
}

// usage prints the flags in their double-dash form. The flag package accepts
// -addr and --addr alike, and the help shows the one the README uses.
func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "agenttik %s - run agent coding sessions from one local UI.\n\n", version)
	fmt.Fprintf(out, "Usage:\n  agenttik [options]\n\nOptions:\n")
	flag.VisitAll(func(f *flag.Flag) {
		placeholder, help := flag.UnquoteUsage(f)
		name := "--" + f.Name
		if placeholder != "" {
			name += " " + placeholder
		}
		fmt.Fprintf(out, "  %-22s %s", name, help)
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(out, " (default %s)", f.DefValue)
		}
		fmt.Fprintln(out)
	})
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
