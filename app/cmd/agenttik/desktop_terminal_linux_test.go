//go:build desktop && production && linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Typing into a terminal posts the keystrokes as the request body, and the
// real Wails/WebKit URI-scheme reader segfaults on some body shapes: a Blob
// body took the whole app down on the first character typed. Drive the actual
// terminals.js through a real window in a subprocess, because that crash is
// native and neither Go nor a browser unit test can catch it.
// Run with make test-desktop-terminal (needs Xvfb and the desktop libraries).
func TestDesktopTerminalInput(t *testing.T) {
	if os.Getenv("AGENTTIK_TERMINAL_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-terminal")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopTerminalInput$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_TERMINAL_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native terminal input failed: %v\n%s", err, output)
		}
		return
	}

	sources := webModules(t, "terminals.js")

	// 0xe9 is a report-mode byte, the one shape that has to reach the shell as
	// a single byte. Typing more while the first request is still in flight is
	// what makes a later one carry several chunks joined into one body.
	want := []byte{'a', 'b', 'c', 0xe9}
	sources["index.html"] = &fstest.MapFile{Data: []byte(fmt.Sprintf(`<script type="importmap">{"imports":{"vue":"/vue.js"}}</script>
<script type="module">
import { sendTerminalInput, sendTerminalBinary } from '/terminals.js';
try {
  sendTerminalInput('shell', 'a');
  sendTerminalInput('shell', 'bc');
  sendTerminalBinary('shell', 'é');
  // The sends are fire-and-forget, so wait for the bytes to land before asking
  // for the verdict. Counting them keeps this out of any decoding question.
  for (let i = 0; i < 100 && (await (await fetch('/typed')).text()) !== '%d'; i++) {
    await new Promise((r) => setTimeout(r, 50));
  }
  await fetch('/result');
} catch (e) { await fetch('/result?error=' + encodeURIComponent(e.message)); }
</script>`, len(want)))}

	started := make(chan context.Context, 1)
	var mu sync.Mutex
	var typed []byte
	app := &options.App{
		Title: "Terminal input regression test",
		Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{
			Assets: sources,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/terminals/shell/input":
					body, err := io.ReadAll(r.Body)
					if err != nil || r.Method != "POST" {
						t.Errorf("input request: method=%s error=%v", r.Method, err)
					}
					mu.Lock()
					typed = append(typed, body...)
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
				case "/typed":
					mu.Lock()
					count := len(typed)
					mu.Unlock()
					fmt.Fprintf(w, "%d", count)
				case "/result":
					mu.Lock()
					got := append([]byte(nil), typed...)
					mu.Unlock()
					if msg := r.URL.Query().Get("error"); msg != "" || !bytes.Equal(got, want) {
						t.Errorf("typed=%x want %x, browser error=%s", got, want, msg)
					}
					runtime.Quit(<-started)
				default:
					http.NotFound(w, r)
				}
			}),
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(typed, want) {
		t.Fatalf("window exited before the keystrokes arrived: %x", typed)
	}
}

// webModules serves real UI modules to a test window, along with everything
// they import. A module that fails to load takes its whole test with it
// silently — the page never reaches the code under test and the window just
// waits — so the import graph is served rather than only the entry point.
func webModules(t *testing.T, names ...string) fstest.MapFS {
	t.Helper()
	// api.js carries apiURL, which every module that talks to the server uses,
	// and diagnostics.js rides along behind it. Neither has a Vue app to
	// report into here, so "vue" resolves to the one function they need.
	sources := fstest.MapFS{
		"vue.js": &fstest.MapFile{Data: []byte("export const ref = (value) => ({ value });\n")},
	}
	for _, name := range append(names, "api.js", "diagnostics.js") {
		source, err := os.ReadFile("../../../web/src/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = &fstest.MapFile{Data: source}
	}
	return sources
}
