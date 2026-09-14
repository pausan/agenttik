package server

import (
	"strings"
	"testing"
)

func TestBranchActions(t *testing.T) {
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
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("commit", "--allow-empty", "-m", "initial")
	git("branch", "merged-main")
	git("switch", "-c", "master")
	git("commit", "--allow-empty", "-m", "master only")
	git("branch", "merged-master")
	git("branch", "in-worktree")
	git("worktree", "add", t.TempDir(), "in-worktree")
	git("switch", "-c", "unmerged", "main")
	git("commit", "--allow-empty", "-m", "keep")
	p, err := st.CreateProject("branches", root)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID)
	request := func(action, branch string, want int) {
		t.Helper()
		r := do(t, s, "POST", base+"/branches/"+action, map[string]string{"branch": branch})
		defer r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("%s: got %d, want %d", action, r.StatusCode, want)
		}
	}
	log := decode[projectLog](t, do(t, s, "GET", base+"/log", nil))
	if len(log.Branches) != 6 {
		t.Fatalf("branches: %v", log.Branches)
	}
	request("clean", "main", 400)
	request("clean", "unmerged", 200)
	if got := git("branch", "--format=%(refname:short)"); got != "in-worktree\nmain\nmaster\nunmerged" {
		t.Fatalf("cleanup left: %s", got)
	}
	request("switch", "--detach", 400)
	request("switch", "missing", 400)
	request("switch", "main", 204)
	if got := git("branch", "--show-current"); got != "main" {
		t.Fatal(got)
	}
	request("push", "main", 400)
	request("pull", "main", 400)
	// Exercise both network actions against a local bare remote.
	remote := t.TempDir()
	if _, err := runGit(remote, "init", "--bare"); err != nil {
		t.Fatal(err)
	}
	git("remote", "add", "origin", remote)
	git("push", "-u", "origin", "main")
	git("config", "push.default", "matching")
	git("commit", "--allow-empty", "-m", "push me")
	request("push", "main", 204)
	out, err := runGit(remote, "rev-parse", "refs/heads/main")
	if err != nil || strings.TrimSpace(out) != git("rev-parse", "HEAD") {
		t.Fatalf("push: %s %v", out, err)
	}
	git("reset", "--hard", "HEAD~1")
	request("pull", "main", 204)
	if git("rev-parse", "HEAD") != strings.TrimSpace(out) {
		t.Fatal("pull did not advance HEAD")
	}
	git("switch", "--detach")
	request("push", "main", 400)
}

func TestCleanupWithoutMainOrMaster(t *testing.T) {
	s, st := newTestServer(t)
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "trunk"},
		{"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"},
		{"branch", "keep"},
	} {
		if _, err := runGit(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	p, err := st.CreateProject("branches", root)
	if err != nil {
		t.Fatal(err)
	}
	body := decode[struct {
		Deleted []string `json:"deleted"`
	}](t,
		do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/branches/clean", map[string]string{"branch": "trunk"}))
	if len(body.Deleted) != 0 {
		t.Fatal(body.Deleted)
	}
	if _, err := runGit(root, "show-ref", "--verify", "refs/heads/keep"); err != nil {
		t.Fatal(err)
	}
}
