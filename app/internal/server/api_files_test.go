package server

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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
