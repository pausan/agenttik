package server

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestWorkspaceRepositories(t *testing.T) {
	s, _ := newTestServer(t)
	root := t.TempDir()
	for _, name := range []string{"alpha", "group/beta", "node_modules/ignored"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := runGit(dir, "init"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := runGit(dir, "add", "."); err != nil {
			t.Fatal(err)
		}
		if _, err := runGit(dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", name); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(name+" changed"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p := decode[store.Project](t, do(t, s, "POST", "/api/projects", map[string]string{"path": root}))
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	repos := decode[[]string](t, do(t, s, "GET", base+"/repositories", nil))
	if !reflect.DeepEqual(repos, []string{"alpha", "group/beta"}) {
		t.Fatalf("repositories: %v", repos)
	}
	for _, repo := range repos {
		changes := decode[[]changedFile](t, do(t, s, "GET", base+"/changes?repo="+repo, nil))
		if len(changes) != 1 || changes[0].Path != repo+"/file.txt" {
			t.Fatalf("changes: %v", changes)
		}
		log := decode[projectLog](t, do(t, s, "GET", base+"/log?repo="+repo, nil))
		if len(log.Commits) != 1 || log.Commits[0].Subject != repo {
			t.Fatalf("log: %v", log)
		}
		detail := decode[commitDetail](t, do(t, s, "GET", base+"/commit?repo="+repo+"&hash="+log.Head, nil))
		if len(detail.Files) != 1 || detail.Files[0].Path != repo+"/file.txt" {
			t.Fatalf("commit: %v", detail)
		}
		for _, endpoint := range []string{"/diff?", "/commit/diff?hash=" + log.Head + "&"} {
			diff := decode[fileDiff](t, do(t, s, "GET", base+endpoint+"path="+repo+"/file.txt", nil))
			if diff.Diff == "" {
				t.Fatalf("empty diff: %s", endpoint)
			}
		}
	}
	for _, query := range []string{"repo=../outside", "repo=node_modules", "path=../outside/file"} {
		resp := do(t, s, "GET", base+"/log?"+query, nil)
		if resp.StatusCode != 400 {
			t.Fatalf("%s: %d", query, resp.StatusCode)
		}
	}
}

func TestRepositoriesEmptySingleAndWorktree(t *testing.T) {
	root := t.TempDir()
	repos, err := repositoriesIn(root)
	if err != nil || len(repos) != 0 {
		t.Fatalf("empty: %v %v", repos, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere"), 0644); err != nil {
		t.Fatal(err)
	}
	repos, err = repositoriesIn(root)
	if err != nil || !reflect.DeepEqual(repos, []string{"."}) {
		t.Fatalf("single worktree: %v %v", repos, err)
	}
}
