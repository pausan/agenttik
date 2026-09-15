package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

type conflictProvider struct {
	fake.Provider
	calls   int
	resolve func(agent.TurnRequest) error
}

func (p *conflictProvider) Run(_ context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	p.calls++
	if err := p.resolve(req); err != nil {
		return nil, err
	}
	events := make(chan agent.Event)
	close(events)
	return events, nil
}

func TestIntegrateBranches(t *testing.T) {
	for _, action := range []string{"merge", "rebase"} {
		for _, conflict := range []bool{false, true} {
			for _, dirty := range []bool{false, true} {
				t.Run(action+map[bool]string{true: " conflicts", false: " clean"}[conflict]+map[bool]string{true: " dirty", false: " pristine"}[dirty], func(t *testing.T) {
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
					write := func(path, value string) {
						t.Helper()
						if err := os.WriteFile(filepath.Join(root, path), []byte(value), 0600); err != nil {
							t.Fatal(err)
						}
					}
					git("init", "-b", "destination")
					git("config", "user.name", "Test")
					git("config", "user.email", "test@example.com")
					write("local.txt", "original\n")
					write("deleted.txt", "delete me\n")
					write("file.txt", "base\n")
					git("add", ".")
					git("commit", "-m", "base")
					git("switch", "-c", "source")
					if conflict {
						write("file.txt", "source\n")
					} else {
						write("source.txt", "source\n")
					}
					git("add", ".")
					git("commit", "-m", "source")
					if conflict && action == "rebase" {
						write("file.txt", "source second\n")
						git("commit", "-am", "source second")
					}
					source := git("rev-parse", "source")
					git("switch", "destination")
					write("file.txt", "destination\n")
					git("commit", "-am", "destination")
					destination := git("rev-parse", "destination")
					git("switch", "source")
					write("saved.txt", "existing stash\n")
					git("stash", "push", "-u")
					existingStash := git("rev-parse", "refs/stash")
					if dirty {
						write("local.txt", "staged\n")
						git("add", "local.txt")
						write("local.txt", "working edit\n")
						write("new.txt", "untracked\n")
						if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
							t.Fatal(err)
						}
					}
					p, err := st.CreateProject("integrate", root)
					if err != nil {
						t.Fatal(err)
					}
					provider := &conflictProvider{}
					provider.resolve = func(req agent.TurnRequest) error {
						if req.Model != "fake-careful" || req.Effort != "high" || req.WorkDir != root || !req.Isolated || req.Permission != agent.PermissionWorkspace {
							t.Fatalf("wrong request: %+v", req)
						}
						write("file.txt", "source and destination resolved\n")
						return nil
					}
					s.registry = agent.NewRegistry(provider)
					s.runner = runner.New(st, s.registry, runner.NewHub())
					choice := store.ActionModel{Provider: "fake", Model: "fake-careful", Effort: "high"}
					if err := st.SetActionModel("merge", &choice); err != nil {
						t.Fatal(err)
					}
					res := do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/branches/"+action, map[string]string{"branch": "source", "target": "destination"})
					if res.StatusCode != 200 {
						b, _ := io.ReadAll(res.Body)
						t.Fatalf("%d %s", res.StatusCode, b)
					}
					if op := gitOperation(root); op != "" {
						t.Fatalf("still %s", op)
					}
					if dirty {
						if git("show", ":local.txt") != "working edit" || git("show", ":new.txt") != "untracked" {
							t.Fatal("local changes were not restored staged")
						}
						if git("diff", "--cached", "--name-status") != "D\tdeleted.txt\nM\tlocal.txt\nA\tnew.txt" {
							t.Fatal("unexpected restored changes")
						}
					} else if git("status", "--porcelain") != "" {
						t.Fatal("dirty after integration")
					}
					if git("rev-parse", "refs/stash") != existingStash {
						t.Fatal("existing stash changed")
					}
					git("diff", "--quiet")
					if conflict && provider.calls == 0 || !conflict && provider.calls != 0 {
						t.Fatalf("model calls: %d", provider.calls)
					}
					if action == "merge" {
						if git("branch", "--show-current") != "destination" || git("rev-parse", "source") != source {
							t.Fatal("wrong merge direction")
						}
						git("merge-base", "--is-ancestor", source, "destination")
						git("merge-base", "--is-ancestor", destination, "destination")
					} else {
						if git("branch", "--show-current") != "source" || git("rev-parse", "destination") != destination {
							t.Fatal("wrong rebase direction")
						}
						git("merge-base", "--is-ancestor", destination, "source")
						if conflict && provider.calls != 2 {
							t.Fatalf("wanted two conflict rounds: %d", provider.calls)
						}
					}
				})
			}
		}
	}
}

func TestIntegrationFailureAndRecovery(t *testing.T) {
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
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "file"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	write("base\n")
	git("add", ".")
	git("commit", "-m", "base")
	git("switch", "-c", "feature")
	write("feature\n")
	git("commit", "-am", "feature")
	git("switch", "main")
	write("main\n")
	git("commit", "-am", "main")
	git("switch", "feature")
	p, _ := st.CreateProject("recovery", root)
	request := func(action, target string, want int) {
		t.Helper()
		res := do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/branches/"+action, map[string]string{"branch": "feature", "target": target})
		if res.StatusCode != want {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s: %d %s", action, res.StatusCode, b)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "local"), []byte("local changes"), 0600); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	git("worktree", "add", worktree, "main")
	request("merge", "main", 400)
	if git("branch", "--show-current") != "feature" {
		t.Fatal("switched before checking target worktree")
	}
	git("worktree", "remove", worktree)
	request("merge", "feature", 400)
	request("merge", "--abort", 400)
	request("merge", "missing", 400)

	unlock, err := s.lockRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	request("merge", "main", 400)
	unlock()
	request("merge", "main", 400) // missing conflict model leaves a recoverable merge
	if gitOperation(root) != "merge" {
		t.Fatal("lost merge state")
	}
	log := decode[projectLog](t, do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/log", nil))
	if log.Operation != "merge" {
		t.Fatal(log.Operation)
	}
	request("switch", "", 400)
	request("abort", "", 200)
	if git("show", ":local") != "local changes" {
		t.Fatal("abort did not restore local changes")
	}
	git("switch", "feature")
	provider := &conflictProvider{resolve: func(req agent.TurnRequest) error { return nil }}
	s.registry = agent.NewRegistry(provider)
	s.runner = runner.New(st, s.registry, runner.NewHub())
	choice := store.ActionModel{Provider: "fake", Model: "fake-careful", Effort: "high"}
	if err := st.SetActionModel("merge", &choice); err != nil {
		t.Fatal(err)
	}
	request("merge", "main", 400) // model claims success without resolving markers
	if gitOperation(root) != "merge" {
		t.Fatal("committed unresolved markers")
	}
	provider.resolve = func(req agent.TurnRequest) error { write("resolved both\n"); return nil }
	request("continue", "", 200)
	if git("show", ":local") != "local changes" {
		t.Fatal("retry did not restore local changes")
	}
	git("commit", "-m", "local changes")
	// Recreate divergence for the rebase timeout/retry case.
	git("switch", "main")
	write("new main\n")
	git("commit", "-am", "new main")

	git("switch", "feature")
	write("new feature\n")
	git("commit", "-am", "new feature")
	provider.resolve = func(req agent.TurnRequest) error { return context.DeadlineExceeded }
	request("rebase", "main", 400)
	if gitOperation(root) != "rebase" {
		t.Fatal("lost rebase state")
	}
	provider.resolve = func(req agent.TurnRequest) error {
		write("resolved both\n")
		return os.WriteFile(filepath.Join(root, "unrelated"), []byte("unrelated edit"), 0600)
	}
	request("continue", "", 400)
	if gitOperation(root) != "rebase" {
		t.Fatal("committed unrelated model changes")
	}
	if err := os.Remove(filepath.Join(root, "unrelated")); err != nil {
		t.Fatal(err)
	}
	provider.resolve = func(req agent.TurnRequest) error { write("resolved both\n"); return nil }
	request("continue", "", 200)
	if gitOperation(root) != "" {
		t.Fatal("retry did not finish")
	}
}

func TestIntegrationStashRestoreConflict(t *testing.T) {
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
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "file"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	write("base\n")
	git("add", ".")
	git("commit", "-m", "base")
	git("switch", "-c", "feature")
	git("switch", "main")
	write("destination\n")
	git("commit", "-am", "destination")
	git("switch", "feature")
	write("local edit\n")
	p, _ := st.CreateProject("restore conflict", root)
	res := do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/branches/merge", map[string]string{"branch": "feature", "target": "main"})
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 400 || !strings.Contains(string(body), "changes remain saved in stash") {
		t.Fatalf("%d %s", res.StatusCode, body)
	}
	if gitOperation(root) != "" || git("show", "stash@{0}:file") != "local edit" {
		t.Fatal("integration did not finish or lost local changes")
	}
}
