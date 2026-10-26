//go:build desktop && production && linux

package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Check native WebKit's secure-origin microphone API without opening hardware
// or downloading/running a speech model.
func TestDesktopMicrophoneAPI(t *testing.T) {
	if os.Getenv("AGENTTIK_MIC_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires Xvfb")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopMicrophoneAPI$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_MIC_TEST_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native microphone API: %v\n%s", err, out)
		}
		return
	}
	started := make(chan context.Context, 1)
	var reported atomic.Bool
	html := `<script>setTimeout(async()=>{const ok=window.isSecureContext&&!!navigator.mediaDevices?.getUserMedia;await fetch('/result?ok='+ok);},500);</script>`
	app := &options.App{Title: "Microphone API test", Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { startNative(nil); started <- ctx },
		AssetServer: &assetserver.Options{Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(html)}},
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/result" {
					http.NotFound(w, r)
					return
				}
				reported.Store(true)
				if r.URL.Query().Get("ok") != "true" {
					t.Error("microphone API unavailable on native origin")
				}
				runtime.Quit(<-started)
			})},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if !reported.Load() {
		t.Fatal("window closed before checking microphone API")
	}
}
