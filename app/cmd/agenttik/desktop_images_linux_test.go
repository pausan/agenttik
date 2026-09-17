//go:build desktop && production && linux

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
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

// Exercise the real Wails/WebKit URI-scheme body reader in a subprocess:
// a native segmentation fault cannot be caught by Go or browser unit tests.
// Run with make test-desktop-images (needs Xvfb and the desktop libraries).
func TestDesktopImageUpload(t *testing.T) {
	if os.Getenv("AGENTTIK_IMAGE_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires an X display; use make test-desktop-images")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopImageUpload$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_IMAGE_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native image upload failed: %v\n%s", err, output)
		}
		return
	}

	sources := webModules(t, "prompt-images.js")
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL1sAAAAASUVORK5CYII="
	want, err := base64.StdEncoding.DecodeString(png)
	if err != nil {
		t.Fatal(err)
	}
	// Both ClipboardItem.getType() and DataTransferItem.getAsFile() reach
	// uploadImage. Test their Blob and File shapes through the actual module.
	html := fmt.Sprintf(`<script type="importmap">{"imports":{"vue":"/vue.js"}}</script>
<script type="module">
import { uploadImage } from '/prompt-images.js';
try {
  const bytes = Uint8Array.from(atob('%s'), c => c.charCodeAt(0));
  for (const file of [new Blob([bytes], {type: 'image/png'}), new File([bytes], 'paste.png', {type: 'image/png'})]) {
    const image = await uploadImage(file);
    if (image.url !== '/api/attachments/test.png') throw new Error('Wrong attachment URL');
  }
  await fetch('/result');
} catch (e) { await fetch('/result?error=' + encodeURIComponent(e.message)); }
</script>`, png)
	sources["index.html"] = &fstest.MapFile{Data: []byte(html)}

	started := make(chan context.Context, 1)
	var uploads atomic.Int32
	app := &options.App{
		Title: "Image upload regression test",
		Width: 400, Height: 300,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{
			Assets: sources,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/attachments":
					body, err := io.ReadAll(r.Body)
					if err != nil || r.Method != "POST" || r.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, want) {
						t.Errorf("image request: method=%s type=%s bytes=%x error=%v", r.Method, r.Header.Get("Content-Type"), body, err)
					}
					uploads.Add(1)
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"url":"/api/attachments/test.png"}`)
				case "/result":
					if msg := r.URL.Query().Get("error"); msg != "" || uploads.Load() != 2 {
						t.Errorf("uploads=%d, browser error=%s", uploads.Load(), msg)
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
	if uploads.Load() != 2 {
		t.Fatalf("window exited before both image uploads completed: %d", uploads.Load())
	}
}
