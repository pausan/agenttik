//go:build desktop && production && linux

package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WebKitGTK's media player cannot read from Wails' custom scheme, so the
// window plays video through the loopback media bridge. Play a real clip both
// ways in the real webview: the window's own origin must still fail (or the
// bridge is no longer needed), the bridge must play and seek.
// Run with make test-desktop-video (needs Xvfb, GStreamer and ffmpeg).
func TestDesktopVideo(t *testing.T) {
	if os.Getenv("AGENTTIK_VIDEO_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-video")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopVideo$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_VIDEO_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native video playback failed: %v\n%s", err, output)
		}
		return
	}

	clip := filepath.Join(t.TempDir(), "clip.webm")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=25:duration=6",
		"-c:v", "libvpx", clip).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	html := `<script type="module">
async function play(src) {
  const v = document.createElement('video');
  v.muted = true;
  v.src = src;
  document.body.append(v);
  try {
    return await new Promise((resolve) => {
      setTimeout(() => resolve('timeout'), 10000);
      v.onerror = () => resolve('error');
      v.onloadedmetadata = () => { v.currentTime = 4; };
      v.onseeked = () => v.play().catch((e) => resolve(String(e)));
      v.ontimeupdate = () => { if (v.currentTime > 4.3) resolve('played'); };
    });
  } finally { v.remove(); }
}
const video = '/api/projects/1/video?path=clip.webm';
const { base } = await (await fetch('/desktop/media')).json();
const result = [await play(video), await play(base + video)].join(',');
await fetch('/result?r=' + result);
</script>`

	started := make(chan context.Context, 1)
	result := ""
	var bridge *mediaBridge
	bridge, stop, err := startMediaBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/projects/1/video":
			f, err := os.Open(clip)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", "video/webm")
			http.ServeContent(w, r, "", time.Time{}, f)
		case "/result":
			result = r.URL.Query().Get("r")
			runtime.Quit(<-started)
		default:
			http.NotFound(w, r)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	app := &options.App{
		Title: "Video playback regression test",
		Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{
			Assets:  fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(html)}},
			Handler: bridge.Handler(),
		},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if result != "error,played" {
		t.Fatalf("custom scheme, bridge = %q, want error,played", result)
	}
}
