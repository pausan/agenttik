package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRawFontTypes(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("fonts", dir)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte{0, 1, 0, 0, 255, 128}
	for _, tc := range []struct{ name, kind string }{
		{"sample.ttf", "font/ttf"},
		{"sample.OTF", "font/otf"},
		{"sample.woff", "font/woff"},
		{"sample.woff2", "font/woff2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, tc.name), data, 0o644); err != nil {
				t.Fatal(err)
			}
			resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/raw?path="+tc.name, nil)
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 200 || !bytes.Equal(got, data) {
				t.Fatalf("font bytes: status %d, body %q, error %v", resp.StatusCode, got, err)
			}
			if resp.Header.Get("Content-Type") != tc.kind || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected headers: %v", resp.Header)
			}
		})
	}
	for _, name := range []string{"sample.svg", "sample.html", "sample.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
		resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/raw?path="+name, nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("raw endpoint accepted %s: %d", name, resp.StatusCode)
		}
	}
}

func TestTreeIncludesIgnoredFilesBeyondFormerLimit(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	if _, err := runGit(dir, "init"); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "cache/\nz-last.log\n")
	write("visible.txt", "visible")
	if err := os.Mkdir(filepath.Join(dir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20001; i++ {
		write(fmt.Sprintf("cache/%05d.txt", i), "")
	}
	write("z-last.log", "ignored")
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject("complete tree", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := decode[projectFiles](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/tree", nil))
	if len(got.Ignored) != 20002 || !slices.Contains(got.Ignored, "z-last.log") {
		t.Fatalf("ignored listing incomplete: %d entries; last file present: %v", len(got.Ignored), slices.Contains(got.Ignored, "z-last.log"))
	}
	if !slices.Contains(got.Files, "visible.txt") {
		t.Error("ordinary file missing")
	}
	if !slices.Contains(got.Dirs, "empty") {
		t.Error("empty folder missing")
	}
}

// A transcript names files the project does not hold — another checkout, a log
// under /tmp — and following that link reads the file where it is. What is
// written, and what a revision can answer for, stay inside the project.
func TestAbsolutePathReadsOutsideTheProject(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("alpha", dir)
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	text := filepath.Join(elsewhere, "notes.txt")
	if err := os.WriteFile(text, []byte("elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	font := filepath.Join(elsewhere, "sample.woff2")
	data := []byte{0, 1, 0, 0, 255, 128}
	if err := os.WriteFile(font, data, 0o644); err != nil {
		t.Fatal(err)
	}
	id := itoa(p.ID)
	at := func(path string) string { return "path=" + url.QueryEscape(path) }

	if got := decode[fileContent](t, do(t, s, "GET", "/api/projects/"+id+"/file?"+at(text), nil)); got.Content != "elsewhere" {
		t.Errorf("content = %q, want the file outside the project", got.Content)
	}
	if got := decode[fileInfo](t, do(t, s, "GET", "/api/projects/"+id+"/file-info?"+at(text), nil)); got.Size != int64(len("elsewhere")) {
		t.Errorf("size = %d, want %d", got.Size, len("elsewhere"))
	}
	resp := do(t, s, "GET", "/api/projects/"+id+"/raw?"+at(font), nil)
	defer resp.Body.Close()
	if got, err := io.ReadAll(resp.Body); err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(got, data) {
		t.Errorf("raw bytes: status %d, body %q, error %v", resp.StatusCode, got, err)
	}

	// Saving is not a read: the project boundary still holds.
	if resp := do(t, s, "PUT", "/api/projects/"+id+"/file?"+at(text), map[string]string{"content": "rewritten"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("save status = %d, want 400", resp.StatusCode)
	}
	if got, _ := os.ReadFile(text); string(got) != "elsewhere" {
		t.Errorf("file outside the project was rewritten: %q", got)
	}
	// A revision is a repository's answer, and no repository holds this file.
	if resp := do(t, s, "GET", "/api/projects/"+id+"/file-info?"+at(text)+"&rev=HEAD", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("revision status = %d, want 400", resp.StatusCode)
	}
}
