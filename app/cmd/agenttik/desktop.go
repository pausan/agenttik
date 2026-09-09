//go:build desktop

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/pausan/agenttik/app/internal/server"
)

// runDesktop opens a native window over the same HTTP server the web mode
// uses. The window talks to a loopback listener through a reverse proxy rather
// than to an in-process handler, because the SSE stream needs a real
// connection it can flush.
func runDesktop(srv *server.Server) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	go srv.Listener(ln)

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
		AssetServer:      &assetserver.Options{Handler: http.Handler(proxy)},
		BackgroundColour: &options.RGBA{R: 17, G: 18, B: 21, A: 255},
		OnShutdown:       func(ctx context.Context) { srv.Shutdown() },
	})
}
