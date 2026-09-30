package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVideoRanges(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("videos", dir)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("0123456789abcdef")
	if err := os.WriteFile(filepath.Join(dir, "clip.MKV"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + itoa(p.ID) + "/video?path=clip.MKV"
	get := func(rng string) (int, string, map[string]string) {
		req := httptest.NewRequest("GET", url, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := s.app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		h := map[string]string{}
		for _, k := range []string{"Content-Type", "Content-Range", "Accept-Ranges", "Cache-Control", "X-Content-Type-Options"} {
			h[k] = resp.Header.Get(k)
		}
		return resp.StatusCode, string(body), h
	}

	code, body, h := get("")
	if code != 200 || body != string(data) || h["Content-Type"] != "video/x-matroska" || h["Accept-Ranges"] != "bytes" ||
		h["Cache-Control"] != "no-store" || h["X-Content-Type-Options"] != "nosniff" {
		t.Fatalf("whole file: %d %q %v", code, body, h)
	}
	for rng, want := range map[string]string{"bytes=2-5": "2345", "bytes=10-": "abcdef", "bytes=-3": "def", "bytes=14-99": "ef"} {
		code, body, h := get(rng)
		if code != 206 || body != want || h["Content-Range"] == "" {
			t.Fatalf("%s: %d %q %v", rng, code, body, h)
		}
	}
	if code, _, h := get("bytes=16-"); code != 416 || h["Content-Range"] != "bytes */16" {
		t.Fatalf("past the end: %d %v", code, h)
	}

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/video?path=notes.txt", nil)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("video endpoint accepted a text file: %d", resp.StatusCode)
	}
}

// A real AVI goes through ffprobe and ffmpeg, where both are installed.
func TestVideoTranscode(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("videos", dir)
	if err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(dir, "clip.avi")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=65x50:rate=10:duration=2",
		"-f", "lavfi", "-i", "sine=duration=2", "-c:v", "mpeg4", "-c:a", "mp3", clip).CombinedOutput(); err != nil {
		t.Skipf("cannot make a test clip: %v: %s", err, out)
	}

	resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/video-info?path=clip.avi", nil)
	var info videoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !info.Transcode || info.Duration < 1.9 || info.Duration > 2.2 || info.Width != 65 || info.Height != 50 || info.Video != "mpeg4" || info.Audio != "mp3" {
		t.Fatalf("info: %+v", info)
	}

	req := httptest.NewRequest("GET", "/api/projects/"+itoa(p.ID)+"/video?path=clip.avi&transcode=1&start=1", nil)
	resp, err = s.app.Test(req, 20000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "video/webm" || len(body) < 1000 || string(body[:4]) != "\x1a\x45\xdf\xa3" {
		t.Fatalf("transcode: %d %s %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	converted := filepath.Join(t.TempDir(), "out.webm")
	if err := os.WriteFile(converted, body, 0o644); err != nil {
		t.Fatal(err)
	}
	var probed videoInfo
	out, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json",
		"-show_entries", "format=duration:stream=codec_type,codec_name,width,height", converted).Output()
	if err != nil {
		t.Fatal(err)
	}
	probed.probe(out)
	// Started a second in, so a second is left; the odd width is rounded.
	if probed.Video != "vp9" || probed.Audio != "opus" || probed.Width != 64 || probed.Duration > 1.3 {
		t.Fatalf("converted: %+v", probed)
	}
}
