package server

import (
	"context"
	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
	"os"
	"path/filepath"
	"strings"
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
	request("commit-message", map[string]string{}, 400) // no provider; index stays intact
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

type commitMessageProvider struct {
	fake.Provider
	request agent.TurnRequest
}

func (p *commitMessageProvider) Run(_ context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	p.request = req
	events := make(chan agent.Event, 1)
	events <- agent.Event{Type: agent.EventText, Text: "Add staged text"}
	close(events)
	return events, nil
}
func TestGenerateCommitMessageUsesOnlyIndex(t *testing.T) {
	s, st := newTestServer(t)
	provider := &commitMessageProvider{}
	s.registry = agent.NewRegistry(provider)
	s.runner = runner.New(st, s.registry, runner.NewHub())
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
	p, err := st.CreateProject("message", root)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/projects/" + itoa(p.ID) + "/commit-message"
	if res := do(t, s, "POST", endpoint, map[string]string{}); res.StatusCode != 400 {
		t.Fatalf("empty index: %d", res.StatusCode)
	}
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("staged text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	if err := os.WriteFile(path, []byte("unstaged secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	choice := store.ActionModel{Provider: "fake", Model: "fake-careful", Effort: "high"}
	if err := st.SetActionModel("commit_message", &choice); err != nil {
		t.Fatal(err)
	}
	before := git("diff", "--cached")
	res := do(t, s, "POST", endpoint, map[string]string{})
	if res.StatusCode != 200 {
		t.Fatalf("generation: %d", res.StatusCode)
	}
	result := decode[map[string]string](t, res)
	if provider.request.Model != "fake-careful" || provider.request.Effort != "high" {
		t.Fatalf("ignored model override: %+v", provider.request)
	}
	if result["message"] != "Add staged text" {
		t.Fatalf("response: %v", result)
	}
	if !strings.Contains(provider.request.Prompt, "+staged text") || strings.Contains(provider.request.Prompt, "unstaged secret") {
		t.Fatalf("wrong diff: %s", provider.request.Prompt)
	}
	if git("diff", "--cached") != before {
		t.Fatal("index changed")
	}
	if _, err := runGit(root, "rev-parse", "--verify", "HEAD"); err == nil {
		t.Fatal("generation committed")
	}
}
