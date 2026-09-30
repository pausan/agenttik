//go:build desktop

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMediaBridgeForwardsOnlyVideoWithItsToken(t *testing.T) {
	var seen []string
	b, stop, err := startMediaBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		io.WriteString(w, "video")
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !strings.HasPrefix(b.base, "http://127.0.0.1:") || len(b.prefix) != len("/media/")+32 {
		t.Fatalf("base %q, prefix %q", b.base, b.prefix)
	}

	get := func(url string) int {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := get(b.base + "/api/projects/3/video?path=a.mkv&profile=work"); code != 200 {
		t.Fatalf("video: %d", code)
	}
	origin := strings.TrimSuffix(b.base, b.prefix)
	for _, url := range []string{
		origin + "/api/projects/3/video?path=a.mkv",
		origin + "/media/00000000000000000000000000000000/api/projects/3/video?path=a.mkv",
		b.base + "/api/projects/3/file?path=secret.txt",
		b.base + "/api/projects/3/raw?path=a.png",
		b.base + "/api/projects/3/video/../file?path=secret.txt",
		b.base + "/desktop/media",
	} {
		if code := get(url); code != 404 {
			t.Errorf("%s: %d", url, code)
		}
	}
	if len(seen) != 1 || seen[0] != "/api/projects/3/video?path=a.mkv&profile=work" {
		t.Fatalf("forwarded %v", seen)
	}

	// The window asks where media is; everything else goes to its handler.
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/desktop/media", nil))
	var answer struct{ Base string }
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || answer.Base != b.base {
		t.Fatalf("media answer %q: %v", rec.Body.String(), err)
	}
	b.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/projects", nil))
	if len(seen) != 2 || seen[1] != "/api/projects" {
		t.Fatalf("window requests: %v", seen)
	}
}
