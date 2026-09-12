package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/single"
)

// callAPI is a client of the instance holding this data directory. It never
// starts an app or opens its database, so a provider can use it mid-turn.
func callAPI(cfg config.Config, method, path, body string, stdin io.Reader, stdout io.Writer) error {
	method = strings.ToUpper(method)
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return fmt.Errorf("unsupported API method %q", method)
	}
	u, err := url.ParseRequestURI(path)
	if err != nil || u.Host != "" || u.Scheme != "" || !strings.HasPrefix(u.Path, "/api/") {
		return errors.New("API path must start with /api/")
	}
	var input io.Reader
	if body == "-" {
		input = stdin
	} else if body != "" {
		input = strings.NewReader(body)
	}
	if input != nil {
		data, err := io.ReadAll(io.LimitReader(input, 4*1024*1024+1))
		if err != nil {
			return err
		}
		if len(data) > 4*1024*1024 || !json.Valid(data) {
			return errors.New("API body must be valid JSON of at most 4 MiB")
		}
		input = strings.NewReader(string(data))
	}
	// Discovery is read-only: the provider may read the data directory while
	// being allowed to write only inside its own project workspace.
	addr := single.Addr(cfg.LockPath())
	if addr == "" {
		return errors.New("agenttik is not running or is still starting for this data directory")
	}
	// A web server may listen on every interface. Connect through loopback.
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			if ip.To4() != nil {
				host = "127.0.0.1"
			} else {
				host = "::1"
			}
			addr = net.JoinHostPort(host, port)
		}
	}
	req, err := http.NewRequest(method, "http://"+addr+u.RequestURI(), input)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reach agenttik: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var answer struct{ Error string }
		if json.NewDecoder(resp.Body).Decode(&answer) == nil && answer.Error != "" {
			return fmt.Errorf("agenttik: %s: %s", resp.Status, answer.Error)
		}
		return fmt.Errorf("agenttik: %s", resp.Status)
	}
	_, err = io.Copy(stdout, resp.Body)
	return err
}
