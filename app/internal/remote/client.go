package remote

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
)

// Client belongs to one desktop window or one loopback-only browser client.
// Each remote connection gets its own cookie jar, so switching servers never
// forwards the previous server's session or the browser's local cookies.
type Client struct {
	handler atomic.Pointer[http.Handler]
	pending atomic.Pointer[http.Handler]
}

func NewClient(initial http.Handler) *Client {
	c := &Client{}
	c.handler.Store(&initial)
	return c
}

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
			if absolute.Scheme != target.Scheme || absolute.Host != target.Host {
				return fmt.Errorf("remote server redirected outside its origin")
			}
			r.Header.Set("Location", absolute.RequestURI())
		}
		return nil
	}
	var handler http.Handler = proxy
	c.handler.Store(&handler)
	return "/"
}

func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == ConnectPath {
		ConnectHandler(func(target *url.URL) string {
			next := NewClient(http.NotFoundHandler())
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
