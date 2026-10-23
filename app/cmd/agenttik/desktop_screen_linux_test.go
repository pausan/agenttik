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
	"strings"
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
// drawn and input comes back. It reports which codec the window chose.
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

	sources := webModules(t, "screen.js", "eventStream.js")
	sources["index.html"] = &fstest.MapFile{Data: []byte(`<canvas id="c"></canvas>
<script type="importmap">{"imports":{"vue":"/vue.js"}}</script>
<script type="module">
import { chooseCodec, ScreenStream } from '/screen.js';
const report = (q) => fetch('/result?' + new URLSearchParams(q));
try {
  const info = await (await fetch('/api/screen')).json();
  if (!info.same_machine) throw new Error('the local desktop must not offer its own Screen button');
  const codec = await chooseCodec(['vp9', 'vp8', 'jpeg']);
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
  await report({ codec, red, secure: String(isSecureContext), clipboard: String(!!navigator.clipboard) });
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
				t.Logf("screen codec %s, secure context %s, clipboard API %s", q.Get("codec"), q.Get("secure"), q.Get("clipboard"))
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
