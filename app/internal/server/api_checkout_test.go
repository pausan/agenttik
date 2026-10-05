package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutCommit(t *testing.T) {
	s, st := newTestServer(t)
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	write("file.txt", "first\n")
	git("add", ".")
	git("commit", "-m", "first")
	first := git("rev-parse", "HEAD")
	write("file.txt", "second\n")
	git("commit", "-am", "second")
	second := git("rev-parse", "HEAD")
	p, err := st.CreateProject("checkout", root)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID)
	request := func(hash string, want int) {
		t.Helper()
		resp := do(t, s, "POST", base+"/branches/checkout", map[string]string{"hash": hash})
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("checkout %q: got %d, want %d", hash, resp.StatusCode, want)
		}
	}
	for _, hash := range []string{"", "--force", "HEAD~1", "deadbeef", git("rev-parse", "HEAD:file.txt")} {
		request(hash, 400)
	}
	unlock, err := s.lockRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	request(first, 400)
	unlock()
	// Checkout must not overwrite local edits or proceed during a merge.
	write("file.txt", "local edit\n")
	request(first, 400)
	if got, err := os.ReadFile(filepath.Join(root, "file.txt")); err != nil || string(got) != "local edit\n" {
		t.Fatalf("local changes lost: %q, %v", got, err)
	}
	git("restore", "file.txt")
	write(".git/MERGE_HEAD", first+"\n")
	request(first, 400)
	if err := os.Remove(filepath.Join(root, ".git/MERGE_HEAD")); err != nil {
		t.Fatal(err)
	}
	if git("rev-parse", "HEAD") != second || git("branch", "--show-current") != "main" {
		t.Fatal("failed checkout changed HEAD")
	}
	request(first[:8], 204)
	log := decode[projectLog](t, do(t, s, "GET", base+"/log", nil))
	if log.Branch != "" || log.Head != first[:8] || len(log.Commits) != 1 {
		t.Fatalf("detached log: %+v", log)
	}
	if got, err := os.ReadFile(filepath.Join(root, "file.txt")); err != nil || string(got) != "first\n" {
		t.Fatalf("checkout content: %q, %v", got, err)
	}
	// A second checkout works while detached; branch tips never move.
	request(second, 204)
	if git("branch", "--show-current") != "" || git("rev-parse", "HEAD") != second || git("rev-parse", "main") != second {
		t.Fatal("checkout did not preserve detached HEAD and branch tip")
	}
	resp := do(t, s, "POST", base+"/branches/switch", map[string]string{"branch": "main"})
	resp.Body.Close()
	if resp.StatusCode != 204 || git("branch", "--show-current") != "main" {
		t.Fatal("could not switch back to main")
	}
}
