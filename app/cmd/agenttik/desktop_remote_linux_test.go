//go:build desktop && production && linux

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/remote"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/bcrypt"
)

// Exercise the real login form through the native window's URI scheme.
func TestDesktopRemoteLogin(t *testing.T) {
	if os.Getenv("AGENTTIK_REMOTE_TEST_CHILD") != "1" {
		if os.Getenv("DISPLAY") == "" {
			t.Skip("requires Xvfb")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopRemoteLogin$", "-test.v")
		cmd.Env = append(os.Environ(), "AGENTTIK_REMOTE_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native remote login failed: %v\n%s", err, output)
		}
		return
	}
	const password = "password &+= spaces"
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	gate := netauth.New(func() netauth.Credentials {
		return netauth.Credentials{Enabled: true, PasswordHash: string(hash), Secret: secret}
	})
	var signedIn atomic.Bool
	protected := gate.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signedIn.Store(true)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<h1>Signed in</h1><script>fetch('/result')</script>")
	}))
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, r)
		t.Logf("server request: %s %s -> %d", r.Method, r.URL.Path, rec.Code)
		for key, values := range rec.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(rec.Code)
		body := rec.Body.String()
		if strings.Contains(body, "<form") && r.Method == "GET" {
			code, _ := netauth.Code(secret, time.Now())
			// Use the real form submit handler, including any production script.
			body = strings.Replace(body, "</body>", fmt.Sprintf(`<script>
window.addEventListener('load', () => {
 document.querySelector('[name=password]').value = %q;
 document.querySelector('[name=code]').value = %q;
 document.querySelector('form').requestSubmit();
});
</script></body>`, password, code), 1)
		}
		io.WriteString(w, body)
	}))
	defer targetServer.Close()
	target, _ := url.Parse(targetServer.URL)
	client := remote.NewClient(http.NotFoundHandler())
	client.UseDesktopNavigation()
	client.Connect(target)

	started := make(chan context.Context, 1)
	app := &options.App{
		Title: "Remote login regression", Width: 400, Height: 400,
		OnStartup: func(ctx context.Context) { started <- ctx },
		AssetServer: &assetserver.Options{Handler: quietAborts(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("window request: %s %s", r.Method, r.URL.Path)
			if r.URL.Path == "/result" {
				if !signedIn.Load() {
					t.Error("login did not reach the protected server")
				}
				runtime.Quit(<-started)
				return
			}
			client.ServeHTTP(w, r)
		}))},
	}
	configureDesktop(app)
	if err := wails.Run(app); err != nil {
		t.Fatal(err)
	}
	if !signedIn.Load() {
		t.Fatal("window exited before login")
	}
}
