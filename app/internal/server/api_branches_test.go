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

func TestRemoteCleanupConfirmation(t *testing.T) {
	s, st := newTestServer(t)
	root, remote := t.TempDir(), t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := runGit(dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	git(remote, "init", "--bare")
	git(root, "init", "-b", "main")
	git(root, "config", "user.name", "Test")
	git(root, "config", "user.email", "test@example.com")
	git(root, "commit", "--allow-empty", "-m", "initial")
	git(root, "branch", "merged")
	git(root, "switch", "-c", "master")
	git(root, "commit", "--allow-empty", "-m", "master")
	git(root, "branch", "merged-master")
	git(root, "switch", "-c", "unmerged", "main")
	git(root, "commit", "--allow-empty", "-m", "unmerged")
	git(root, "switch", "main")
	git(root, "remote", "add", "origin", remote)
	git(root, "push", "origin", "main", "master", "merged", "merged-master", "unmerged")
	// The remote main can be ahead of the local main; discovery must fetch it.
	git(root, "switch", "-c", "remote-only")
	git(root, "commit", "--allow-empty", "-m", "remote main advance")
	git(root, "push", "origin", "remote-only", "HEAD:main")
	git(root, "switch", "main")
	p, err := st.CreateProject("remote cleanup", root)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID) + "/branches/"
	decode[map[string]any](t, do(t, s, "POST", base+"clean", map[string]string{"branch": "main"}))
	git(remote, "show-ref", "--verify", "refs/heads/merged") // No consent yet.
	if _, err := runGit(root, "show-ref", "--verify", "refs/heads/merged"); err == nil {
		t.Fatal("local merged branch survived")
	}
	checked := decode[struct {
		Remotes []remoteBranch `json:"remotes"`
	}](t, do(t, s, "POST", base+"check-remote", map[string]string{"branch": "main"}))
	if len(checked.Remotes) != 3 {
		t.Fatalf("candidates: %+v", checked.Remotes)
	}
	// A candidate updated while the dialog is open must survive confirmation.
	git(root, "push", "origin", "unmerged:merged")
	response := do(t, s, "POST", base+"clean-remote", map[string]any{"branch": "main", "remotes": checked.Remotes})
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("stale confirmation status: %d", response.StatusCode)
	}
	git(remote, "show-ref", "--verify", "refs/heads/merged-master")
	checked = decode[struct {
		Remotes []remoteBranch `json:"remotes"`
	}](t, do(t, s, "POST", base+"check-remote", map[string]string{"branch": "main"}))
	result := decode[struct {
		Deleted []string `json:"deleted"`
		Error   string   `json:"error"`
	}](t, do(t, s, "POST", base+"clean-remote", map[string]any{"branch": "main", "remotes": checked.Remotes}))
	if result.Error != "" || len(result.Deleted) != 2 {
		t.Fatalf("delete: %+v", result)
	}
	for _, name := range []string{"merged-master", "remote-only"} {
		if _, err := runGit(remote, "show-ref", "--verify", "refs/heads/"+name); err == nil {
			t.Fatalf("remote %s survived", name)
		}
	}
	for _, name := range []string{"main", "master", "unmerged", "merged"} {
		git(remote, "show-ref", "--verify", "refs/heads/"+name)
	}
	// User-supplied protected refs are never accepted.
	response = do(t, s, "POST", base+"clean-remote", map[string]any{"branch": "main", "remotes": []remoteBranch{{"origin", "main", git(remote, "rev-parse", "refs/heads/main")}}})
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("protected branch status: %d", response.StatusCode)
	}
}

func TestRemoteCleanupUsesEachRemotesBases(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := runGit(dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	git(root, "init", "-b", "main")
	git(root, "config", "user.name", "Test")
	git(root, "config", "user.email", "test@example.com")
	git(root, "commit", "--allow-empty", "-m", "initial")
	first := git(root, "rev-parse", "HEAD")
	git(root, "commit", "--allow-empty", "-m", "feature")
	for _, name := range []string{"origin", "other", "no-base"} {
		remote := t.TempDir()
		git(remote, "init", "--bare")
		git(root, "remote", "add", name, remote)
		git(root, "push", name, "HEAD:feature")
		if name == "origin" {
			git(root, "push", name, first+":refs/heads/main")
		}
		if name == "other" {
			git(root, "push", name, "HEAD:main")
		}
	}
	candidates, err := mergedRemoteBranches(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Remote != "other" || candidates[0].Branch != "feature" {
		t.Fatalf("cross-remote candidates: %+v", candidates)
	}
	git(root, "remote", "set-url", "--push", "origin", t.TempDir())
	if _, err := mergedRemoteBranches(root); err == nil {
		t.Fatal("accepted different push destination")
	}
}
