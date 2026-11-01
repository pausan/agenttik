//go:build desktop && production && linux

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Exercise the real task rows in WebKitGTK, including native key events. A
// browser test cannot detect a crash of the desktop process in GTK's loop.
func TestDesktopTaskKeyboard(t *testing.T) {
	if os.Getenv("AGENTTIK_TASK_KEYS_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires Xvfb and xdotool")
		}
		if _, err := exec.LookPath("xdotool"); err != nil {
			t.Skip("requires xdotool")
		}
		for _, status := range []string{"running", "idle"} {
			t.Run(status, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopTaskKeyboard$", "-test.v")
				cmd.Env = append(os.Environ(), "AGENTTIK_TASK_KEYS_CHILD=1", "AGENTTIK_TASK_KEYS_STATUS="+status,
					"XDG_DATA_HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "XDG_CACHE_HOME="+t.TempDir())
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("native task keyboard: %v\n%s", err, output)
				}
			})
		}
		return
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("Keyboard", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(&store.Session{ID: "keyboard", ProjectID: project.ID, Provider: "fake", Title: "Keyboard task"}); err != nil {
		t.Fatal(err)
	}
	registry := agent.NewRegistry()
	srv := server.New(db, registry, runner.New(db, registry, runner.NewHub()))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Listener(ln)
	defer srv.Shutdown()
	if err := db.SetSessionStatus("keyboard", os.Getenv("AGENTTIK_TASK_KEYS_STATUS")); err != nil {
		t.Fatal(err)
	}
	reports := make(chan string, 1)
	var verified atomic.Bool

	target, _ := url.Parse("http://" + ln.Addr().String())
	proxy := localDesktopProxy(target, "", "")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-check" {
			reports <- r.URL.Query().Get("value")
			w.WriteHeader(204)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	app := &options.App{
		Title: "Task keyboard regression", Width: 1000, Height: 700,
		AssetServer: &assetserver.Options{Handler: handler},
		OnStartup:   func(context.Context) { startNative(nil) },
		OnDomReady: func(ctx context.Context) {
			go func() {
				defer runtime.Quit(ctx)
				// Wait for Vue to render and for each key's effect. Failing to find a
				// task must fail the test rather than silently typing into an empty page.
				check := func(condition string) bool {
					runtime.WindowExecJS(ctx, fmt.Sprintf(`(async () => {
      for (let i = 0; i < 30; i++) {
       if (%s) { await fetch('/test-check?value=ok'); return; }
       await new Promise(r => setTimeout(r, 50));
      }
      await fetch('/test-check?value=timeout');
     })()`, condition))
					select {
					case value := <-reports:
						if value == "ok" {
							return true
						}
						t.Errorf("page condition failed: %s", condition)
					case <-time.After(8 * time.Second):
						t.Errorf("page stopped answering: %s", condition)
					}
					return false
				}
				if !check(`!!document.querySelector('.task-row button[aria-label="Mark unread"]')`) {
					return
				}
				wantPulse := os.Getenv("AGENTTIK_TASK_KEYS_STATUS") == "running"
				if !check(fmt.Sprintf(`!!document.querySelector('.task-row .status-pulse') === %t`, wantPulse)) {
					return
				}
				id, err := exec.Command("xdotool", "search", "--name", "^Task keyboard regression$").Output()
				if err != nil {
					t.Error(err)
					return
				}
				if output, err := exec.Command("xdotool", "windowfocus", strings.TrimSpace(string(id))).CombinedOutput(); err != nil {
					t.Errorf("focus: %v %s", err, output)
					return
				}
				press := func(keys ...string) bool {
					if output, err := exec.Command("xdotool", append([]string{"key", "--delay", "15"}, keys...)...).CombinedOutput(); err != nil {
						t.Errorf("keys %v: %v %s", keys, err, output)
						return false
					}
					return true
				}
				runtime.WindowExecJS(ctx, `document.querySelector('.task-row button').focus()`)
				if !check(`document.activeElement.matches('.task-row button[aria-label="Mark unread"]')`) {
					return
				}
				for i := 0; i < 3; i++ {
					for _, label := range []string{"Mark star", "Mark warning", "Mark danger", "Clear mark", "Mark unread"} {
						if !press("space") || !check(fmt.Sprintf(`document.activeElement.getAttribute('aria-label') === %q`, label)) {
							return
						}
						// Flag writes refresh both lists asynchronously. Let those
						// responses settle before the next deliberate cycle step.
						time.Sleep(200 * time.Millisecond)
					}
				}
				// Open the task and return focus to its dot before further keys.
				runtime.WindowExecJS(ctx, `document.querySelector('.task-select').click()`)
				if !check(`!!document.querySelector('textarea')`) {
					return
				}
				runtime.WindowExecJS(ctx, `document.querySelector('.task-row button').focus()`)
				if !check(`document.activeElement.matches('.task-row button')`) {
					return
				}
				if !press("space", "Tab", "Menu", "Down", "Escape", "F2", "Escape", "F6", "F7", "F10", "Escape", "BackSpace") {
					return
				}
				if !check(`!!document.querySelector('.task-row button')`) {
					return
				}
				verified.Store(true)
			}()
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if !verified.Load() {
		t.Error("window exited before task keyboard checks completed")
	}
}
