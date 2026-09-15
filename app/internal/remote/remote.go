// Package remote verifies an instance before a client opens its UI.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const VersionPath = "/api/version"
const CheckPath = "/api/remote/check"

const ConnectPath = "/api/remote/connect"

type Info struct {
	Name        string `json:"name,omitempty"`
	Application string `json:"application"`
	Version     string `json:"version"`
}

// Parse accepts an origin only. Credentials belong in the server's login form.
func Parse(address string) (*url.URL, error) {
	address = strings.TrimSpace(address)
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, errors.New("enter a host:port or an HTTP(S) URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("enter a host:port or an HTTP(S) URL without credentials, a path, query, or fragment")
	}
	if strings.HasSuffix(u.Host, ":") || (strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(u.Host, "[")) {
		return nil, errors.New("enter a valid port; IPv6 hosts must use brackets")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("port must be between 1 and 65535")
		}
	}
	u.Path = ""
	return u, nil
}

func Check(ctx context.Context, address string) (*url.URL, Info, error) {
	u, err := Parse(address)
	if err != nil {
		return nil, Info{}, err
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+VersionPath, nil)
	if err != nil {
		return nil, Info{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, Info{}, fmt.Errorf("could not reach server: %w", err)
	}
	defer res.Body.Close()
	var info Info
	body, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(body) > 4096 || res.StatusCode != http.StatusOK || json.Unmarshal(body, &info) != nil || info.Application != "agenttik" || strings.TrimSpace(info.Version) == "" {
		return nil, Info{}, errors.New("this URL did not identify itself as an agenttik instance at /api/version")
	}
	return u, info, nil
}

// ConnectHandler checks before invoking connect. Browsers navigate to the
// returned origin; desktop clients replace their window's proxy and reload.
func ConnectHandler(connect func(*url.URL) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fail := func(status int, err string) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"error": err})
		}
		if r.Method != http.MethodPost {
			fail(405, "use POST")
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			fail(415, "use application/json")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				fail(403, "cross-origin connection request refused")
				return
			}
		}
		var input struct {
			Address string `json:"address"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			fail(400, "invalid connection request")
			return
		}
		target, info, err := Check(r.Context(), input.Address)
		if err != nil {
			fail(400, err.Error())
			return
		}
		json.NewEncoder(w).Encode(struct {
			URL     string `json:"url"`
			Version string `json:"version"`
			Name    string `json:"name,omitempty"`
		}{connect(target), info.Version, info.Name})
	})
}
