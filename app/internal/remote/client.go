package remote

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
)

// Client belongs to one desktop window or one loopback-only browser client.
// Each remote connection gets its own cookie jar, so switching servers never
// forwards the previous server's session or the browser's local cookies.
type Client struct {
	handler           atomic.Pointer[http.Handler]
	pending           atomic.Pointer[http.Handler]
	desktopNavigation bool
}

func NewClient(initial http.Handler) *Client {
	c := &Client{}
	c.handler.Store(&initial)
	return c
}

// UseDesktopNavigation must be called before serving requests. WebKit custom
// schemes do not follow HTTP redirects, so login navigation needs an HTML page.
func (c *Client) UseDesktopNavigation() { c.desktopNavigation = true }

func (c *Client) Connect(target *url.URL) string {
	jar, _ := cookiejar.New(nil)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
		r.Header.Del("Origin")
		r.Header.Del("Referer")
		for _, cookie := range jar.Cookies(target) {
			r.AddCookie(cookie)
		}
	}
	proxy.ModifyResponse = func(r *http.Response) error {
		r.Header.Set("X-Agenttik-Remote", target.String())
		jar.SetCookies(target, r.Cookies())
		r.Header.Del("Set-Cookie")
		// Keep authentication redirects inside this window's proxy.
		if location := r.Header.Get("Location"); location != "" {
			u, err := url.Parse(location)
			if err != nil {
				return err
			}
			absolute := target.ResolveReference(u)
			if absolute.Scheme != target.Scheme || absolute.Host != target.Host || strings.HasPrefix(absolute.RequestURI(), "//") {
				return fmt.Errorf("remote server redirected outside its origin")
			}
			r.Header.Set("Location", absolute.RequestURI())
			if c.desktopNavigation && r.StatusCode == http.StatusSeeOther && strings.Contains(r.Request.Header.Get("Accept"), "text/html") {
				path := absolute.RequestURI()
				destination, _ := json.Marshal(path)
				body := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><title>agenttik</title><script>window.location.replace(%s)</script><a href="%s">Continue</a>`, destination, html.EscapeString(path))
				r.Body.Close()
				r.Body = io.NopCloser(strings.NewReader(body))
				r.StatusCode = http.StatusOK
				r.Status = "200 OK"
				r.ContentLength = int64(len(body))
				r.TransferEncoding = nil
				r.Header.Del("Location")
				r.Header.Del("Content-Encoding")
				r.Header.Set("Content-Type", "text/html; charset=utf-8")
				r.Header.Set("Content-Length", strconv.Itoa(len(body)))
				r.Header.Set("Cache-Control", "no-store")
			}
		}
		return nil
	}
	var handler http.Handler = proxy
	c.handler.Store(&handler)
	return "/"
}

func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == CheckPath {
		ConnectHandler(func(target *url.URL) string { return target.String() + "/" }).ServeHTTP(w, r)
		return
	}
	if r.URL.Path == ConnectPath {
		ConnectHandler(func(target *url.URL) string {
			next := NewClient(http.NotFoundHandler())
			next.desktopNavigation = c.desktopNavigation
			next.Connect(target)
			c.pending.Store(next.handler.Load())
			return "/"
		}).ServeHTTP(w, r)
		return
	}
	// Switch on navigation, after the old UI has saved its state. Background
	// requests made while discovery finishes still reach the old instance.
	if r.Method == http.MethodGet && r.URL.Path == "/" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		if pending := c.pending.Swap(nil); pending != nil {
			c.handler.Store(pending)
		}
	}
	(*c.handler.Load()).ServeHTTP(w, r)
}
