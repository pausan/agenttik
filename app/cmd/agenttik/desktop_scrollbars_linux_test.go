//go:build desktop && production && linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pausan/agenttik/web"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Pixel checks are necessary: WebKit's native scrollbars can paint above an
// overlay even when DOM hit testing reports the overlay on top.
func TestDesktopScrollbarLayers(t *testing.T) {
	if os.Getenv("AGENTTIK_SCROLLBAR_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires Xvfb; use make test-desktop-scrollbars")
		}
		for _, tool := range []string{"xdotool", "import"} {
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("requires %s", tool)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopScrollbarLayers$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_SCROLLBAR_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native scrollbar layers: %v\n%s", err, output)
		}
		return
	}
	styles, err := fs.Glob(web.Assets(), "assets/index-*.css")
	if err != nil || len(styles) != 1 {
		t.Fatalf("build the UI first: CSS=%v error=%v", styles, err)
	}
	css, err := fs.ReadFile(web.Assets(), styles[0])
	if err != nil {
		t.Fatal(err)
	}
	platform, err := os.ReadFile("../../../web/src/platform.js")
	if err != nil {
		t.Fatal(err)
	}
	const html = `<!doctype html><html><head><link rel="stylesheet" href="/app.css">
<style>
/* The Vue plugin normally supplies the neutral palette aliases. */
:root { --ui-color-neutral-400: #a1a1aa; --ui-color-neutral-500: #71717a; }
body { margin: 0; background: white; }
#pane { width: 200px; height: 240px; background: #eee; }
#filler { width: 2000px; height: 2000px; }
#cover { position: fixed; left: 160px; top: 40px; width: 160px; height: 240px; background: #ff00ff; }
</style></head><body>
<template id="fixture"><div id="shell" class="app-shell"><div id="pane" class="overflow-auto"><div id="filler"></div></div></div>
<div id="cover" class="z-50"></div></template>
<script type="module">
import { needsCSSScrollbars } from '/platform.js';
document.documentElement.classList.toggle('desktop-linux', needsCSSScrollbars());
// Apply the class before creating scrollers, just as main.js does before mounting.
document.body.append(document.getElementById('fixture').content.cloneNode(true));
const pane = document.getElementById('pane');
// Keep overlay scrollbars visible while the screenshot is taken.
const timer = setInterval(() => {
  pane.scrollTop = pane.scrollTop === 0 ? 10 : 0;
  pane.scrollLeft = pane.scrollTop;
}, 50);
for (const dark of [false, true]) {
  document.documentElement.classList.toggle('dark', dark);
  for (const nested of [false, true]) {
    const shell = document.getElementById('shell');
    shell.style.position = nested ? 'fixed' : '';
    shell.style.zIndex = nested ? '50' : '';
    await new Promise(resolve => setTimeout(resolve, 300));
    await fetch('/capture?case=' + (dark ? 'dark' : 'light') + (nested ? '-nested' : '-pane'));
  }
}
clearInterval(timer);
await fetch('/done');
</script></body></html>`
	started := make(chan context.Context, 1)
	var captured atomic.Int32
	app := &options.App{
		Title: "Scrollbar layer regression", Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{
			Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(html)}, "app.css": &fstest.MapFile{Data: css}, "platform.js": &fstest.MapFile{Data: platform}},
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/done" {
					runtime.Quit(<-started)
					return
				}
				if r.URL.Path != "/capture" {
					http.NotFound(w, r)
					return
				}
				id, err := exec.Command("xdotool", "search", "--name", "^Scrollbar layer regression$").Output()
				if err != nil {
					t.Errorf("find window: %v", err)
					return
				}
				window := strings.TrimSpace(string(id))
				if output, err := exec.Command("xdotool", "mousemove", "--window", window, "198", "20").CombinedOutput(); err != nil {
					t.Errorf("hover scrollbar: %v: %s", err, output)
					return
				}
				time.Sleep(150 * time.Millisecond)
				shot, err := exec.Command("import", "-window", window, "png:-").Output()
				if err != nil {
					t.Errorf("screenshot: %v", err)
					return
				}
				img, err := png.Decode(bytes.NewReader(shot))
				if err != nil {
					t.Errorf("decode screenshot: %v", err)
					return
				}
				captured.Add(1)
				// An uncovered part of the thumb must still be visible.
				red, green, blue, _ := img.At(195, 15).RGBA()
				if red >= 60000 && green >= 60000 && blue >= 60000 {
					t.Errorf("scrollbar thumb is missing: (%d,%d,%d)", red, green, blue)
				}
				// The cover crosses both scrollbars at x=200 and y=240. Every interior pixel must
				// be opaque magenta, including the scrollbar's track and thumb.
				for y := 45; y < 275; y++ {
					for x := 165; x < 315; x++ {
						red, green, blue, _ := img.At(x, y).RGBA()
						if red != 65535 || green != 0 || blue != 65535 {
							path := fmt.Sprintf("/tmp/agenttik-scrollbar-%d-%s.png", os.Getpid(), r.URL.Query().Get("case"))
							_ = os.WriteFile(path, shot, 0600)
							t.Errorf("overlay pixel (%d,%d) = (%d,%d,%d), want opaque magenta; screenshot: %s", x, y, red, green, blue, path)
							return
						}
					}
				}
			}),
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if captured.Load() != 4 {
		t.Errorf("captured %d cases, want 4", captured.Load())
	}
}
