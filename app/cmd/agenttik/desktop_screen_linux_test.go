//go:build desktop && production && linux

package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/screen"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/store"
)

// The desktop window is where most Screen tabs will be opened, and WebKitGTK
// is the browser least like the others: WebCodecs exists only in a secure
// context, and the window's own scheme decides whether it is one. This runs
// the real screen.js in a real window, through the same loopback proxy the
// app uses, against a server with a fake screen, and checks the screen is
// drawn and input comes back. It reports which codec the window chose and
// whether it can play the sound.
// Run with make test-desktop-screen (needs Xvfb and the desktop libraries).
func TestDesktopScreen(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-screen")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopScreen$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_SCREEN_TEST_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("native screen view failed: %v\n%s", err, output)
		}
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "screen codec") {
				t.Log(strings.TrimSpace(line))
			}
		}
		return
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetScreenEnabled(true); err != nil {
		t.Fatal(err)
	}
	registry := agent.NewRegistry()
	srv := server.New(db, registry, runner.New(db, registry, runner.NewHub()))
	fake := screen.NewFake(320, 200)
	fake.Fill(0, 0, 320, 200, 255, 0, 0)
	srv.UseScreen(func() (screen.Display, error) { return fake, nil })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Listener(ln)
	defer srv.Shutdown()
	target, _ := url.Parse("http://" + ln.Addr().String())
	proxy := localDesktopProxy(target, "", "")

	sources := webModules(t, "screen.js", "clipboard.js", "eventStream.js")
	sources["index.html"] = &fstest.MapFile{Data: []byte(`<canvas id="c"></canvas>
<script type="importmap">{"imports":{"vue":"/vue.js"}}</script>
<script type="module">
import { audioSupported, chooseCodec, ScreenStream } from '/screen.js';
const report = (q) => fetch('/result?' + new URLSearchParams(q));
try {
  const info = await (await fetch('/api/screen')).json();
  if (!info.same_machine) throw new Error('the local desktop must not offer its own Screen button');
  const codec = await chooseCodec(info.codecs);
  const canvas = document.getElementById('c');
  const stream = new ScreenStream(canvas, codec, {});
  let red = false;
  for (let i = 0; i < 200 && !red; i++) {
    await new Promise((r) => setTimeout(r, 50));
    if (!canvas.width) continue;
    const [r, g, b] = canvas.getContext('2d').getImageData(10, 10, 1, 1).data;
    red = r > 200 && g < 60 && b < 60;
  }
  stream.input.push({ t: 'm', x: 7, y: 9 });
  await new Promise((r) => setTimeout(r, 500));
  stream.close();
  await report({ codec, red, secure: String(isSecureContext), clipboard: String(!!navigator.clipboard), sound: String(await audioSupported()) });
} catch (e) { await report({ error: e.message }); }
</script>`)}

	started := make(chan context.Context, 1)
	app := &options.App{
		Title: "Screen view regression test",
		Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{
			Assets: sources,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/result" {
					proxy.ServeHTTP(w, r)
					return
				}
				q := r.URL.Query()
				t.Logf("screen codec %s, secure context %s, clipboard API %s, plays sound %s", q.Get("codec"), q.Get("secure"), q.Get("clipboard"), q.Get("sound"))
				if msg := q.Get("error"); msg != "" {
					t.Errorf("browser error: %s", msg)
				}
				if q.Get("red") != "true" {
					t.Errorf("the screen was not drawn (codec %s)", q.Get("codec"))
				}
				moved := false
				for _, line := range fake.Log() {
					moved = moved || line == "move 7 9"
				}
				if !moved {
					t.Errorf("input did not come back: %q", fake.Log())
				}
				w.WriteHeader(http.StatusNoContent)
				runtime.Quit(<-started)
			}),
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
}

// In full screen every key belongs to the other machine, including the ones
// the window manager keeps for itself. A root passive grab on Alt+Tab is how
// a window manager gets that chord, so it stands in for one here: it must stay
// quiet while the window holds the keyboard, the page must see the keys, and
// it must fire again once the window lets go.
// Run with make test-desktop-screen (needs Xvfb, xdotool and the desktop libraries).
func TestDesktopScreenKeyboard(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-screen")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopScreenKeyboard$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_SCREEN_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native keyboard grab failed: %v\n%s", err, output)
		}
		return
	}

	var fired atomic.Int32
	stop, err := registerToggle("Alt+Tab", func(bool) { fired.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	press := func(chord string) {
		t.Helper()
		if output, err := exec.Command("xdotool", "key", chord).CombinedOutput(); err != nil {
			t.Fatalf("xdotool %s: %v\n%s", chord, err, output)
		}
		time.Sleep(200 * time.Millisecond)
	}
	var mu sync.Mutex
	var seen []string
	pageSaw := func(key string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(seen, key)
	}

	started := make(chan context.Context, 1)
	app := &options.App{
		Title: "Screen keyboard regression test",
		Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) {
			watchScreenFullscreen(ctx)
			started <- ctx
		},
		AssetServer: &assetserver.Options{
			Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<textarea id="keys" autofocus></textarea>
<script>
const keys = document.getElementById('keys');
keys.addEventListener('keydown', (e) => {
  e.preventDefault();
  fetch('/key?' + new URLSearchParams({ key: e.key, code: e.code }));
});
window.addEventListener('load', async () => {
  keys.focus();
  await fetch('/ready');
});
</script>`)}},
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/key":
					mu.Lock()
					seen = append(seen, r.URL.Query().Get("key"), r.URL.Query().Get("code"))
					mu.Unlock()
				case "/ready":
					ctx := <-started
					go func() {
						defer runtime.Quit(ctx)
						time.Sleep(500 * time.Millisecond)
						press("alt+Tab")
						if fired.Load() != 1 {
							t.Errorf("the stand-in window manager did not see Alt+Tab before the grab: %d", fired.Load())
						}
						runtime.EventsEmit(ctx, "agenttik:screen-fullscreen", true)
						time.Sleep(time.Second)
						press("alt+Tab")
						press("Escape")
						press("Super_L")
						press("ctrl+w")
						if fired.Load() != 1 {
							t.Errorf("Alt+Tab reached the window manager in full screen: %d", fired.Load())
						}
						for _, key := range []string{"Tab", "Escape", "Super", "w"} {
							if !pageSaw(key) {
								t.Errorf("the page did not see %s", key)
							}
						}
						leaveScreenFullscreen(ctx)
						time.Sleep(time.Second)
						press("alt+Tab")
						if fired.Load() != 2 {
							t.Errorf("Alt+Tab did not come back after full screen: %d", fired.Load())
						}
						mu.Lock()
						t.Logf("page saw %q", seen)
						mu.Unlock()
					}()
				}
				w.WriteHeader(http.StatusNoContent)
			}),
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
}
