//go:build desktop && production && linux

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The desktop uses a custom URI scheme. Exercise relative resources from a
// real opaque WebKit iframe as well as the HTTP browser regression.
func TestDesktopHTMLPreview(t *testing.T) {
	if os.Getenv("AGENTTIK_HTML_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-html")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopHTMLPreview$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_HTML_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native HTML preview failed: %v\n%s", err, output)
		}
		return
	}

	sources := webModules(t, "html-preview.js")
	sources["index.html"] = &fstest.MapFile{Data: []byte(`<script type="module">
import { htmlPreviewURL, htmlPreviewDocument } from '/html-preview.js';
try {
  const frame = document.createElement('iframe');
  frame.setAttribute('sandbox', '');
  const text = '<!doctype html><link rel="stylesheet" href="styles/main.css"><h1>Mockup</h1><img src="images/sample.png"><img src="images/icon.svg"><script>fetch("/executed")<\/script>';
  frame.srcdoc = htmlPreviewDocument(text, new URL(htmlPreviewURL({projectID: 1, path: 'overview.html'}), location.href).href);
  document.body.append(frame);
  let loaded = '0';
  for (let i = 0; i < 100 && loaded !== '5'; i++) {
    await new Promise(resolve => setTimeout(resolve, 50));
    loaded = await (await fetch('/loaded')).text();
  }
  if (loaded !== '5') throw new Error('Only ' + loaded + ' of 5 preview resources loaded');
  if (frame.contentDocument !== null) throw new Error('Preview gained same-origin access');
  await fetch('/result');
} catch (e) { await fetch('/result?error=' + encodeURIComponent(e.message)); }
</script>`)}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL1sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile("../../../e2e/font-fixtures/sample.woff2")
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string]struct {
		kind string
		data []byte
	}{
		"styles/main.css":     {"text/css", []byte(`@import "theme.css"; body { background-image: url("../images/sample.png"); }`)},
		"styles/theme.css":    {"text/css", []byte(`@font-face { font-family: Mockup; src: url("sample.woff2"); } h1 { font-family: Mockup; color: blue; }`)},
		"styles/sample.woff2": {"font/woff2", font},
		"images/sample.png":   {"image/png", png},
		"images/icon.svg":     {"image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><rect width="20" height="10"/></svg>`)},
	}
	started := make(chan context.Context, 1)
	var mu sync.Mutex
	loaded := map[string]bool{}
	finished, executed := false, false
	app := &options.App{
		Title: "HTML preview regression test", Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{Assets: sources, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			path := strings.TrimPrefix(r.URL.Path, "/api/html/-/default/projects/1/preview/-/")
			if asset, ok := assets[path]; ok {
				loaded[path] = true
				w.Header().Set("Content-Type", asset.kind)
				w.Header().Set("Content-Security-Policy", "sandbox; script-src 'none'; object-src 'none'")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if strings.HasPrefix(asset.kind, "font/") {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.Write(asset.data)
				return
			}
			switch r.URL.Path {
			case "/loaded":
				fmt.Fprint(w, len(loaded))
			case "/executed":
				executed = true
			case "/result":
				finished = true
				if msg := r.URL.Query().Get("error"); msg != "" || len(loaded) != len(assets) || executed {
					t.Errorf("loaded=%v, script executed=%v, browser error=%s", loaded, executed, msg)
				}
				runtime.Quit(<-started)
			default:
				http.NotFound(w, r)
			}
		})},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !finished {
		t.Fatal("window exited before preview verification")
	}
}
