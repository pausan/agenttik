package server

import (
	"encoding/base64"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestHTMLPreviewAssets(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("mockup", dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/html/-/default/projects/" + itoa(p.ID) + "/preview/-/"
	for _, tc := range []struct{ name, kind string }{
		{"image #1.png", "image/png"}, {"image.svg", "image/svg+xml"},
		{"style.css", "text/css; charset=utf-8"}, {"font.woff2", "font/woff2"},
		{"page.html", "text/html; charset=utf-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, tc.name), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
			resp := do(t, s, "GET", base+url.PathEscape(tc.name)+"?v=1", nil)
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 || string(body) != "fixture" || resp.Header.Get("Content-Type") != tc.kind {
				t.Fatalf("asset: %d %s %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
			}
			if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") != "sandbox; script-src 'none'; object-src 'none'" {
				t.Fatalf("asset headers: %v", resp.Header)
			}
			cors := ""
			if strings.HasPrefix(tc.kind, "font/") {
				cors = "*"
			}
			if resp.Header.Get("Access-Control-Allow-Origin") != cors {
				t.Fatalf("unexpected CORS header for %s", tc.kind)
			}
		})
	}
	for _, path := range []string{"code.js", "secret.txt", "../escape.css", "%2e%2e/escape.css", "/absolute.css"} {
		resp := do(t, s, "GET", base+path, nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("accepted %s: %d", path, resp.StatusCode)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "style.css"), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "style.css"), filepath.Join(dir, "escape.css")); err == nil {
		resp := do(t, s, "GET", base+"escape.css", nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatal("symlink escaped preview root")
		}
	}
	scope := base64.RawURLEncoding.EncodeToString([]byte(outside))
	resp := do(t, s, "GET", strings.Replace(base, "/preview/-/", "/preview/"+scope+"/", 1)+"style.css", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "external" {
		t.Fatalf("external document asset: %d %q", resp.StatusCode, body)
	}
	large, err := os.Create(filepath.Join(dir, "large.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(maxRawBytes + 1); err != nil {
		t.Fatal(err)
	}
	large.Close()
	resp = do(t, s, "GET", base+"large.png", nil)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal("oversized asset accepted")
	}
}

func TestHTMLPreviewRemoteProfileScope(t *testing.T) {
	remote := profileServer(t)
	profile := decode[Profile](t, do(t, remote, "POST", "/api/profiles", map[string]string{"name": "Mockups"}))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.css"), []byte("remote profile CSS"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := decode[store.Project](t, do(t, remote, "POST", "/api/projects?profile="+profile.ID, map[string]string{"path": dir}))
	local, _ := newTestServer(t)
	state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": listen(t, remote)}))
	if state.Status != "ok" {
		t.Fatalf("remote: %+v", state)
	}
	resp := do(t, local, "GET", "/api/html/"+state.Remote.ID+"/"+profile.ID+"/projects/"+itoa(p.ID)+"/preview/-/main.css", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "remote profile CSS" {
		t.Fatalf("remote profile asset: %d %q", resp.StatusCode, body)
	}
}
