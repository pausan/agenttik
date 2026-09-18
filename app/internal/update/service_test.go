package update

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func release(tag, platform string) Release {
	name := "agenttik_" + path.Base(tag) + "_" + platform
	return Release{Tag: tag, Assets: []Asset{{Name: name, URL: "https://github.com/pausan/agenttik/releases/download/" + tag + "/" + name, Size: 3, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("new")))}}}
}
func TestSelectOffer(t *testing.T) {
	releases := []Release{release("v1.9.0", "linux_amd64"), release("v1.10.0", "linux_amd64"), release("v2.0.0", "windows_amd64.exe"), release("v3.0.0-rc.1", "linux_amd64")}
	draft := release("v4.0.0", "linux_amd64")
	draft.Draft = true
	pre := release("v5.0.0", "linux_amd64")
	pre.Prerelease = true
	releases = append(releases, draft, pre)
	for _, tc := range []struct {
		current, os, arch, want string
		bundle                  bool
	}{
		{"1.8.0", "linux", "amd64", "v1.10.0", false},
		{"1.10.0", "linux", "amd64", "", false},
		{"2.0.0", "linux", "amd64", "", false},
		{"dev", "linux", "amd64", "", false},
		{"abcdef123456", "linux", "amd64", "", false},
		{"1.0.0", "linux", "arm64", "", false},
		{"1.0.0", "windows", "amd64", "v2.0.0", false},
		{"1.0.0", "darwin", "arm64", "", true},
	} {
		t.Run(tc.current+tc.os+tc.arch, func(t *testing.T) {
			got := selectOffer(releases, tc.current, tc.os, tc.arch, tc.bundle)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("unexpected offer: %+v", got)
				}
			} else if got == nil || got.Version != tc.want {
				t.Fatalf("offer: %+v, want %s", got, tc.want)
			}
		})
	}
	bundle := release("v2.0.0", "darwin_arm64.app.zip")
	if selectOffer([]Release{bundle}, "1.0.0", "darwin", "arm64", true) == nil {
		t.Fatal("missing app bundle")
	}
	for _, change := range []func(*Asset){func(a *Asset) { a.Digest = "" }, func(a *Asset) { a.URL = "https://evil.example/binary" }, func(a *Asset) { a.Size = maxDownload + 1 }} {
		r := release("v2.0.0", "linux_amd64")
		change(&r.Assets[0])
		if selectOffer([]Release{r}, "1.0.0", "linux", "amd64", false) != nil {
			t.Fatal("accepted unsafe asset")
		}
	}
}
func TestCheckPersistsIgnoreAndDailyAttempt(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		platform += ".exe"
	}
	r := release("v2.0.0", platform)
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode([]Release{r})
	}))
	defer endpoint.Close()
	s.endpoint = endpoint.URL
	s.check(context.Background())
	if s.Status().Available == nil {
		t.Fatal("no offer")
	}
	if err := s.Ignore("v2.0.0"); err != nil {
		t.Fatal(err)
	}
	again, err := New(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if again.Status().Available != nil {
		t.Fatal("ignored version reappeared")
	}
	again.endpoint = endpoint.URL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	again.Run(ctx)
	if requests.Load() != 1 {
		t.Fatal("rechecked within a day")
	}
	r = release("v2.1.0", platform)
	again.check(context.Background())
	if again.Status().Available == nil || again.Status().Available.Version != "v2.1.0" {
		t.Fatal("ignore hid a later release")
	}
}
func TestFailedCheckIsThrottled(t *testing.T) {
	s, _ := New(t.TempDir(), "1.0.0")
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer endpoint.Close()
	s.endpoint = endpoint.URL
	s.check(context.Background())
	if time.Since(s.state.Checked) > time.Second {
		t.Fatal("attempt not recorded")
	}
}
func TestDownloadIntegrity(t *testing.T) {
	for _, body := range []string{"new", "bad", "new extra", "ne"} {
		t.Run(body, func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer endpoint.Close()
			s, _ := New(t.TempDir(), "1.0.0")
			asset := release("v2.0.0", "linux_amd64").Assets[0]
			asset.URL = endpoint.URL
			err := s.download(context.Background(), asset, filepath.Join(t.TempDir(), "binary"))
			if (err == nil) != (body == "new") {
				t.Fatalf("download error: %v", err)
			}
		})
	}
}
func TestReplaceAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		target := filepath.Join(dir, "app")
		next := filepath.Join(dir, "next")
		backup := filepath.Join(dir, "previous")
		os.WriteFile(target, []byte("old"), 0755)
		if !fail {
			os.WriteFile(next, []byte("new"), 0755)
		}
		err := replace(target, next, backup)
		if (err != nil) != fail {
			t.Fatalf("replace: %v", err)
		}
		data, _ := os.ReadFile(target)
		want := "new"
		if fail {
			want = "old"
		}
		if string(data) != want {
			t.Fatalf("got %q want %q", data, want)
		}
	}
}
func TestTamperedSavedOfferCannotInstall(t *testing.T) {
	s, _ := New(t.TempDir(), "1.0.0")
	s.state.Offer = &Offer{Version: "v2.0.0", Asset: Asset{URL: "https://evil.example/binary"}}
	if err := s.Start(context.Background(), "v2.0.0"); err == nil {
		t.Fatal("accepted tampered offer")
	}
}

func TestHelperRejectsChangedDownload(t *testing.T) {
	dir := t.TempDir()
	target, archive, manifest := filepath.Join(dir, "app"), filepath.Join(dir, "download"), filepath.Join(dir, "plan.json")
	os.WriteFile(target, []byte("old"), 0755)
	os.WriteFile(archive, []byte("bad"), 0600)
	p := Plan{Parent: os.Getpid(), Target: target, Archive: archive, Digest: release("v2.0.0", "linux_amd64").Assets[0].Digest}
	data, _ := json.Marshal(p)
	os.WriteFile(manifest, data, 0600)
	if err := Helper(manifest); err == nil {
		t.Fatal("installed a changed download")
	}
	data, _ = os.ReadFile(target)
	if string(data) != "old" {
		t.Fatal("changed original")
	}
	if _, err := os.Stat(manifest + ".ready"); !os.IsNotExist(err) {
		t.Fatal("reported readiness")
	}
}

func TestNamespacedRelease(t *testing.T) {
	for _, escaped := range []bool{false, true} {
		r := release("v2/v2.1.0", "linux_amd64")
		if escaped {
			r.Assets[0].URL = "https://github.com/pausan/agenttik/releases/download/" + url.PathEscape(r.Tag) + "/" + r.Assets[0].Name
		}
		offer := selectOffer([]Release{release("v1.9.0", "linux_amd64"), r, release("v3/v3.0.0-rc.1", "linux_amd64")}, "2.0.0", "linux", "amd64", false)
		if offer == nil || offer.Version != r.Tag {
			t.Fatalf("escaped=%v: offer=%+v", escaped, offer)
		}
	}
}
