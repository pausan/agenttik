package server

import (
	"slices"
	"strings"
	"testing"
)

func TestPushRequiresConfirmationWithoutUpstream(t *testing.T) {
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
	git(root, "init", "-b", "feature/topic")
	git(root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	git(root, "remote", "add", "origin", remote)
	p, err := st.CreateProject("push confirmation", root)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + itoa(p.ID) + "/branches/push"
	check := func() {
		t.Helper()
		result := decode[struct {
			ConfirmPush bool `json:"confirmPush"`
		}](t, do(t, s, "POST", url, map[string]string{"branch": "feature/topic"}))
		if !result.ConfirmPush {
			t.Fatal("missing confirmation")
		}
		if _, err := runGit(remote, "show-ref", "--verify", "refs/heads/feature/topic"); err == nil {
			t.Fatal("pushed without consent")
		}
	}
	check()
	// Consent captured for a branch that is no longer current cannot push.
	git(root, "switch", "-c", "other")
	r := do(t, s, "POST", url, map[string]any{"branch": "feature/topic", "setUpstream": true})
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal(r.StatusCode)
	}
	git(root, "switch", "feature/topic")
	r = do(t, s, "POST", url, map[string]any{"branch": "feature/topic", "setUpstream": true})
	r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal(r.StatusCode)
	}
	if git(root, "rev-parse", "--abbrev-ref", "@{upstream}") != "origin/feature/topic" {
		t.Fatal("upstream not set")
	}
	if git(remote, "for-each-ref", "--format=%(refname:short)", "refs/heads/") != "feature/topic" {
		t.Fatal("pushed an unrelated branch")
	}
	// Deletion on the remote still asks, even with a cached upstream ref.
	git(remote, "update-ref", "-d", "refs/heads/feature/topic")
	check()
}

func TestPushOffersForceAfterRebase(t *testing.T) {
	s, st := newTestServer(t)
	root, remote, other := t.TempDir(), t.TempDir(), t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := runGit(dir, append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	git(remote, "init", "--bare")
	git(root, "init", "-b", "topic")
	git(root, "commit", "--allow-empty", "-m", "initial")
	git(root, "remote", "add", "origin", remote)
	git(root, "push", "--set-upstream", "origin", "topic")
	git(root, "commit", "--amend", "--allow-empty", "-m", "rebased")
	p, err := st.CreateProject("force push", root)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + itoa(p.ID) + "/branches/push"
	type answer struct {
		ConfirmForce bool   `json:"confirmForce"`
		Remote       string `json:"remote"`
		Tip          string `json:"tip"`
		Error        string `json:"error"`
	}
	asked := decode[answer](t, do(t, s, "POST", url, map[string]string{"branch": "topic"}))
	if !asked.ConfirmForce || asked.Remote != "origin" || asked.Tip != git(remote, "rev-parse", "topic") || asked.Error == "" {
		t.Fatalf("no force confirmation: %+v", asked)
	}
	// The remote moves on after the user was asked: the lease refuses, and a
	// fresh push asks again with the tip it does not know locally.
	git(other, "clone", "-b", "topic", remote, ".")
	git(other, "commit", "--allow-empty", "-m", "elsewhere")
	git(other, "push")
	r := do(t, s, "POST", url, map[string]string{"branch": "topic", "force": asked.Tip})
	r.Body.Close()
	if r.StatusCode != 400 || git(remote, "rev-parse", "topic") != git(other, "rev-parse", "HEAD") {
		t.Fatal("force push overwrote commits the user was not shown")
	}
	asked = decode[answer](t, do(t, s, "POST", url, map[string]string{"branch": "topic"}))
	if !asked.ConfirmForce || asked.Tip != git(other, "rev-parse", "HEAD") {
		t.Fatalf("no force confirmation for unknown tip: %+v", asked)
	}
	r = do(t, s, "POST", url, map[string]string{"branch": "topic", "force": asked.Tip})
	r.Body.Close()
	if r.StatusCode != 204 || git(remote, "rev-parse", "topic") != git(root, "rev-parse", "HEAD") {
		t.Fatal("force push did not replace the remote branch")
	}
	// Other failures stay errors.
	git(root, "remote", "set-url", "origin", t.TempDir())
	r = do(t, s, "POST", url, map[string]string{"branch": "topic"})
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal(r.StatusCode)
	}
}

func TestSwitchAndPullRemoteBranch(t *testing.T) {
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
	initial := git(root, "rev-parse", "HEAD")
	git(root, "remote", "add", "origin", remote)
	git(root, "remote", "add", "other", remote)
	git(root, "config", "remote.other.fetch", "+refs/heads/main:refs/remotes/other/main")
	git(root, "push", "origin", "HEAD:feature/topic")
	p, err := st.CreateProject("remote selection", root)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + itoa(p.ID)
	request := func(action string, body map[string]string, want int) {
		t.Helper()
		r := do(t, s, "POST", url+"/branches/"+action, body)
		r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("%s: %d, want %d", action, r.StatusCode, want)
		}
	}
	git(root, "switch", "--detach")
	request("fetch", map[string]string{}, 204)
	git(root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/feature/topic")
	log := decode[projectLog](t, do(t, s, "GET", url+"/log", nil))
	if !slices.Equal(log.Branches, []string{"main"}) || len(log.RemoteBranches) != 2 || log.RemoteBranches[0].Remote != "origin" || log.RemoteBranches[0].Branch != "feature/topic" || log.RemoteBranches[1].Remote != "other" {
		t.Fatalf("branch lists: %+v %+v", log.Branches, log.RemoteBranches)
	}
	request("switch", map[string]string{"branch": "feature/topic", "remote": "origin"}, 204)
	if git(root, "branch", "--show-current") != "feature/topic" || git(root, "rev-parse", "--abbrev-ref", "@{upstream}") != "origin/feature/topic" {
		t.Fatal("remote selection did not create a local tracking branch")
	}
	git(root, "switch", "main")
	git(root, "commit", "--allow-empty", "-m", "remote advance")
	advanced := git(root, "rev-parse", "HEAD")
	git(root, "push", "origin", "HEAD:feature/topic")
	request("switch", map[string]string{"branch": "feature/topic", "remote": "origin"}, 204)
	if git(root, "rev-parse", "HEAD") != advanced {
		t.Fatal("existing local branch did not fast-forward")
	}
	git(root, "commit", "--allow-empty", "-m", "local commit")
	localTip := git(root, "rev-parse", "HEAD")
	git(root, "switch", "main")
	git(root, "commit", "--allow-empty", "-m", "divergent remote")
	git(root, "push", "origin", "HEAD:feature/topic")
	request("switch", map[string]string{"branch": "feature/topic", "remote": "origin"}, 400)
	if git(root, "rev-parse", "HEAD") != localTip {
		t.Fatal("divergent pull overwrote local commits")
	}
	request("switch", map[string]string{"branch": "missing", "remote": "origin"}, 400)
	request("switch", map[string]string{"branch": "--detach", "remote": "origin"}, 400)
	request("switch", map[string]string{"branch": "main", "remote": "--upload-pack=evil"}, 400)
	request("switch", map[string]string{"branch": "main", "remote": "missing"}, 400)
	if git(root, "rev-parse", "main~2") != initial {
		t.Fatal("main changed unexpectedly")
	}
	git(root, "push", "origin", "main", "main:feature/narrow")
	request("switch", map[string]string{"branch": "feature/narrow", "remote": "other"}, 204)
	if git(root, "rev-parse", "--abbrev-ref", "@{upstream}") != "other/feature/narrow" {
		t.Fatal("narrow fetch configuration did not get a working upstream")
	}
	request("pull", map[string]string{"branch": "feature/narrow"}, 204)
}

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
