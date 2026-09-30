//go:build desktop

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// mediaPath is all the bridge forwards: the video stream, nothing else.
var mediaPath = regexp.MustCompile(`^/api/projects/\d+/video$`)

// mediaBridge serves video to the window over real HTTP. WebKitGTK's media
// player cannot read from a custom URI scheme at all, whatever the response
// says, so a <video> pointed at the window's own origin never plays. The
// bridge is a loopback listener that forwards video requests to the window's
// own handler — so profiles and remote connections behave as in the window —
// behind a random path prefix no other local process or web page can know.
type mediaBridge struct {
	prefix string
	base   string
	next   http.Handler
}

func startMediaBridge(next http.Handler) (*mediaBridge, func(), error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("listen for media: %w", err)
	}
	prefix := "/media/" + hex.EncodeToString(token)
	b := &mediaBridge{prefix: prefix, base: "http://" + ln.Addr().String() + prefix, next: next}
	srv := &http.Server{Handler: http.HandlerFunc(b.serveMedia), ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return b, func() { srv.Close() }, nil
}

func (b *mediaBridge) serveMedia(w http.ResponseWriter, r *http.Request) {
	rest, ok := strings.CutPrefix(r.URL.Path, b.prefix)
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) || !mediaPath.MatchString(rest) {
		http.NotFound(w, r)
		return
	}
	forward := r.Clone(r.Context())
	forward.URL.Path, forward.URL.RawPath, forward.RequestURI = rest, "", ""
	b.next.ServeHTTP(w, forward)
}

// Handler is the window's handler, answering where its media is served from.
func (b *mediaBridge) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/desktop/media" {
			b.next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]string{"base": b.base})
	})
}
