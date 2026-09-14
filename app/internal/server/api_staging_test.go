package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStagingAndCommit(t *testing.T) {
	s, st := newTestServer(t)
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	git("init")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	p, err := st.CreateProject("staging", root)
	if err != nil {
		t.Fatal(err)
	}
	request := func(action string, body any, want int) {
		t.Helper()
		r := do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/"+action, body)
		defer r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("%s: got %d, want %d", action, r.StatusCode, want)
		}
	}
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	name := "a -> \"b\".txt"
	write(name, "first\n")
	request("stage", map[string]string{"path": name}, 204)
	request("unstage", map[string]string{"path": name}, 204)
	if _, err := os.Stat(filepath.Join(root, name)); err != nil {
		t.Fatal("unstaging removed working file", err)
	}
	request("stage", map[string]string{}, 204)
	request("commit", map[string]string{"message": " "}, 400)
	request("commit", map[string]string{"message": "Initial"}, 204)
	write(name, "second\n")
	request("stage", map[string]string{"path": name}, 204)
	write(name, "third\n")
	changes := parseStatus(git("status", "--porcelain=v1", "-z"))
	if len(changes) != 1 || !changes[0].Staged || !changes[0].Unstaged {
		t.Fatalf("partial staging: %+v", changes)
	}
	request("commit", map[string]string{"message": "Second"}, 204)
	if got := git("show", "HEAD:"+name); got != "second\n" {
		t.Fatalf("committed unstaged data: %q", got)
	}
	request("stage", map[string]string{}, 204)
	request("unstage", map[string]string{}, 204)
	if got := git("diff", "--cached"); got != "" {
		t.Fatalf("index still dirty: %s", got)
	}
	request("stage", map[string]string{"path": "../outside"}, 400)
	git("checkout", "--", name)
	git("mv", name, "renamed.txt")
	write("renamed.txt", "second\nmore\n")
	request("stage", map[string]string{"path": "renamed.txt"}, 204)
	request("unstage", map[string]string{"path": "renamed.txt"}, 204)
	if got := git("diff", "--cached"); got != "" {
		t.Fatalf("rename still staged: %s", got)
	}
}
