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
	token := func(root string) string {
		return decode[map[string]string](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/preview-token?root="+root, nil))["token"]
	}
	base := "/api/html/-/default/projects/" + itoa(p.ID) + "/preview/" + token("-") + "/-/"
	for _, tc := range []struct{ name, kind string }{
		{"image #1.png", "image/png"}, {"image.svg", "image/svg+xml"},
		{"style.css", "text/css; charset=utf-8"}, {"font.woff2", "font/woff2"},
		{"page.html", "text/html; charset=utf-8"}, {"chart.js", "text/javascript; charset=utf-8"},
		{"data.json", "application/json"}, {"rows.csv", "text/csv; charset=utf-8"},
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
			if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") != "sandbox allow-scripts; object-src 'none'" {
				t.Fatalf("asset headers: %v", resp.Header)
			}
			// Scripts and fonts load from the page's opaque origin.
			if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
				t.Fatalf("missing CORS header for %s", tc.kind)
			}
		})
	}
	for _, path := range []string{"binary.exe", "notes", "../escape.css", "%2e%2e/escape.css", "/absolute.css"} {
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
	resp := do(t, s, "GET", strings.Replace(base, "/"+token("-")+"/-/", "/"+token(scope)+"/"+scope+"/", 1)+"style.css", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "external" {
		t.Fatalf("external document asset: %d %q", resp.StatusCode, body)
	}
	// A token opens one root: a page cannot reuse its own for another folder,
	// and a guessed one opens nothing.
	for _, bad := range []string{
		strings.Replace(base, "/"+token("-")+"/-/", "/"+token("-")+"/"+scope+"/", 1) + "style.css",
		strings.Replace(base, token("-"), "guess", 1) + "style.css",
	} {
		resp := do(t, s, "GET", bad, nil)
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("token accepted for %s: %d", bad, resp.StatusCode)
		}
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
	token := decode[map[string]string](t, do(t, local, "GET", "/api/projects/"+itoa(p.ID)+"/preview-token?root=-&remote="+state.Remote.ID+"&profile="+profile.ID, nil))["token"]
	resp := do(t, local, "GET", "/api/html/"+state.Remote.ID+"/"+profile.ID+"/projects/"+itoa(p.ID)+"/preview/"+token+"/-/main.css", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "remote profile CSS" {
		t.Fatalf("remote profile asset: %d %q", resp.StatusCode, body)
	}
}
