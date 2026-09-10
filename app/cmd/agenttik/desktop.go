//go:build desktop

package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/pausan/agenttik/app/internal/netserver"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
	"github.com/pausan/agenttik/web"
)

// runDesktop opens a native window over the same HTTP server the web mode
// uses. The window talks to a loopback listener through a reverse proxy rather
// than to an in-process handler, because the SSE stream needs a real
// connection it can flush. defaultAddr seeds Settings › Server: the app's own
// default, or --addr if one was given, for whenever nothing has been saved
// yet.
func runDesktop(srv *server.Server, lock *single.Lock, defaultAddr string) error {
	// A second launch reaches this instance over the same port the window
	// does, so the hook goes in before anything is serving on it.
	win := &window{}
	srv.OnForeground(win.present)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	if err := lock.Publish(ln.Addr().String()); err != nil {
		return err
	}
	go srv.Listener(ln)

	// Settings › Server can additionally expose this same server on a chosen
	// host and port, for a browser rather than only the window — see
	// specs/043-exposed-server.md. It proxies to the same loopback listener
	// the window itself talks to, so the two are never out of step.
	defaultHost, defaultPort, err := net.SplitHostPort(defaultAddr)
	if err != nil {
		defaultHost, defaultPort = "127.0.0.1", "7717"
	}
	port, err := strconv.Atoi(defaultPort)
	if err != nil {
		port = 7717
	}
	srv.SetNetworkManager(netserver.New(ln.Addr().String()), defaultHost, port)
	srv.ApplyStoredNetworkConfig()

	target, err := url.Parse("http://" + ln.Addr().String())
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1 // flush immediately, so SSE streams

	return wails.Run(&options.App{
		Title:            "agenttik",
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Handler: quietAborts(proxy)},
		BackgroundColour: &options.RGBA{R: 17, G: 18, B: 21, A: 255},
		OnStartup:        win.opened,
		OnShutdown:       func(ctx context.Context) { srv.Shutdown() },
		Linux: &linux.Options{
			Icon: appIcon(),
			// Wails only picks this default while Linux options are nil, and
			// the webview goes blank on some drivers with acceleration on.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	})
}

// window is the open Wails window, or the wait for one. The server is already
// answering by the time Wails calls opened, and a second launch can ask to be
// raised in that gap, so the context crosses goroutines and needs the guard.
type window struct {
	mu  sync.Mutex
	ctx context.Context
}

func (w *window) opened(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
}

// present raises the window and says whether there was one. Both calls hand
// the work to the GTK main loop, so any goroutine may make them: on Linux
// Wails maps WindowShow to gtk_widget_show, which covers a hidden window, and
// WindowUnminimise to gtk_window_present, which is the raise and the focus.
func (w *window) present() bool {
	w.mu.Lock()
	ctx := w.ctx
	w.mu.Unlock()
	if ctx == nil {
		return false
	}
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
	return true
}

// quietAborts drops the abort a reverse proxy raises when the client goes away
// mid-body. The webview does that on every reload and on every reopen of the
// event stream, which the UI does whenever a tab opens or closes.
//
// The context value is what makes the proxy raise that abort in the first
// place. Wails runs the handler itself rather than through an http.Server, and
// with no server in the context the proxy assumes it is under test and logs
// every dropped connection instead. Recovering the abort is what an
// http.Server does, so the window stays as quiet as --web.
func quietAborts(h http.Handler) http.Handler {
	// Only has to be non-nil; nothing on this path reads it back.
	srv := &http.Server{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			// Anything else is a real bug, so let it keep unwinding.
			if p := recover(); p != nil && p != http.ErrAbortHandler {
				panic(p)
			}
		}()
		ctx := context.WithValue(r.Context(), http.ServerContextKey, srv)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// appIcon is the logo the browser tab already shows, taken from the embedded
// UI so there is one copy of it. GTK draws it through gdk-pixbuf, which reads
// SVG only with the loader librsvg installs; without it, or without a built
// UI, the window keeps the toolkit default.
func appIcon() []byte {
	svg, err := fs.ReadFile(web.Assets(), "agenttik.svg")
	if err != nil {
		return nil
	}
	return svg
}
