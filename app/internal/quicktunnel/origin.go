package quicktunnel

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// upgradeHeader is how the edge marks a stream that is not a plain request:
// the control stream, or a websocket.
const upgradeHeader = "Cf-Cloudflared-Proxy-Connection-Upgrade"

type websocketKey struct{}

// serve answers one visitor request the edge brought down the connection.
func (c *Client) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Header.Get(upgradeHeader) == "websocket" {
		ctx = context.WithValue(ctx, websocketKey{}, true)
	}
	r.Header.Del(upgradeHeader)
	// Every request arrives from the edge, so the socket's address says
	// nothing about the visitor; the edge's header does, and only the edge
	// can set it on this connection. The lock counts failed logins by it.
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		r.RemoteAddr = net.JoinHostPort(ip, "0")
	}
	rw := &responseWriter{w: w, header: http.Header{}}
	c.handler.ServeHTTP(rw, r.WithContext(ctx))
	rw.WriteHeader(http.StatusOK) // no-op unless the handler wrote nothing
}

// origin is the local server behind the tunnel: a reverse proxy to it, and
// the bridge that carries its event streams over websockets (see bridge.go).
type origin struct {
	target string
	proxy  *httputil.ReverseProxy
}

func newOrigin(target string) *origin {
	u := &url.URL{Scheme: "http", Host: target}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1 // flush at once, so SSE still streams
	return &origin{target: target, proxy: proxy}
}

func (o *origin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Context().Value(websocketKey{}) != nil {
		o.bridge(w, r)
		return
	}
	o.proxy.ServeHTTP(w, r)
}

// responseWriter hands the origin's headers to the edge the way it asked for
// them at registration (serialized_headers): every one of them base64-encoded
// into a single header, so HTTP/2's stricter rules on header names and values
// are not applied to an HTTP/1 origin's.
type responseWriter struct {
	w      http.ResponseWriter
	header http.Header
	wrote  bool
}

func (rw *responseWriter) Header() http.Header { return rw.header }

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wrote {
		return
	}
	rw.wrote = true
	dst := rw.w.Header()
	cl := rw.header.Get("Content-Length")
	if cl != "" {
		dst.Set("Content-Length", cl) // the edge reads this one from HTTP/2 itself
	}
	dst.Set("Cf-Cloudflared-Response-Headers", serializeHeaders(rw.header))
	dst.Set("Cf-Cloudflared-Response-Meta", `{"src":"origin"}`)
	if status == http.StatusSwitchingProtocols {
		status = http.StatusOK // HTTP/2 has no 101; the edge reads 200 as accepted
	}
	rw.w.WriteHeader(status)
	if cl == "" {
		rw.Flush() // a stream: let the visitor see the headers before the first chunk
	}
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	rw.WriteHeader(http.StatusOK)
	return rw.w.Write(p)
}

func (rw *responseWriter) Flush() { rw.w.(http.Flusher).Flush() }

var headerEncoding = base64.RawStdEncoding

// serializeHeaders is cloudflared's connection.SerializeHeaders:
// "b64(name):b64(value)" pairs joined by ";", leaving out the headers that
// carry the tunnel's own signalling.
func serializeHeaders(h http.Header) string {
	var b strings.Builder
	for name, values := range h {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, ":") || strings.HasPrefix(lower, "cf-int-") ||
			strings.HasPrefix(lower, "cf-cloudflared-") || strings.HasPrefix(lower, "cf-proxy-") {
			continue
		}
		for _, v := range values {
			if b.Len() > 0 {
				b.WriteByte(';')
			}
			b.WriteString(headerEncoding.EncodeToString([]byte(name)))
			b.WriteByte(':')
			b.WriteString(headerEncoding.EncodeToString([]byte(v)))
		}
	}
	return b.String()
}
