package server

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLogGraphAndFileCounts(t *testing.T) {
	s, st := newTestServer(t)
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(root, args...)
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(out)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	write("initial.txt", "initial\n")
	git("add", ".")
	git("commit", "-m", "initial")
	baseHash := shortHash(git("rev-parse", "HEAD"))
	git("switch", "-c", "feature")
	write("feature.txt", "feature\n")
	write("binary.dat", "\x00\x01")
	git("add", ".")
	message := "feature\n\nFull message body.\n\n99 files changed\nLast paragraph."
	git("commit", "-m", message)
	git("tag", "-a", "v1", "-m", "release")
	git("tag", "lightweight")
	git("switch", "main")
	git("mv", "initial.txt", "renamed.txt")
	git("commit", "-m", "rename")
	git("merge", "--no-ff", "feature", "-m", "merge")
	head := shortHash(git("rev-parse", "HEAD"))
	git("branch", "side", baseHash)
	git("switch", "side")
	git("commit", "--allow-empty", "-m", "side only")
	git("update-ref", "refs/remotes/origin/side", "HEAD")
	git("switch", "main")
	p, err := st.CreateProject("log", root)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + itoa(p.ID)
	simple := decode[projectLog](t, do(t, s, "GET", url+"/log", nil))
	graph := decode[projectLog](t, do(t, s, "GET", url+"/log?graph=true", nil))
	if len(simple.Commits) != 4 || len(graph.Commits) != 5 {
		t.Fatalf("simple=%d graph=%d", len(simple.Commits), len(graph.Commits))
	}
	if graph.Head != head {
		t.Fatalf("head: %s", graph.Head)
	}
	for _, log := range []projectLog{simple, graph} {
		for _, c := range log.Commits {
			want := c.Subject
			if c.Subject == "feature" {
				want = message
			}
			if c.Message != want {
				t.Fatalf("message: got %q, want %q", c.Message, want)
			}
		}
	}
	positions := map[string]int{}
	for i, c := range graph.Commits {
		positions[c.Hash] = i
	}
	for i, c := range graph.Commits {
		for _, parent := range c.Parents {
			if positions[parent] <= i {
				t.Fatalf("parent precedes child: %v", c)
			}
		}
		want := map[string]int{"initial": 1, "feature": 2, "rename": 1, "merge": 2, "side only": 0}[c.Subject]
		detail := decode[commitDetail](t, do(t, s, "GET", url+"/commit?hash="+c.Hash, nil))
		if c.FileCount != want || len(detail.Files) != want {
			t.Fatalf("%s: count=%d files=%d want=%d", c.Subject, c.FileCount, len(detail.Files), want)
		}
		if c.Subject == "feature" && (!slices.Contains(c.Tags, "v1") || !slices.Contains(c.Tags, "lightweight") || !slices.Contains(c.Branches, "feature")) {
			t.Fatalf("refs: %+v", c)
		}
		if c.Subject == "side only" && !slices.Contains(c.Branches, "origin/side") {
			t.Fatalf("remote: %+v", c)
		}
		if c.Subject == "merge" && len(c.Parents) != 2 {
			t.Fatalf("merge: %+v", c)
		}
	}
	limited := decode[projectLog](t, do(t, s, "GET", url+"/log?graph=true&limit=1", nil))
	if len(limited.Commits) != 1 {
		t.Fatalf("limit: %+v", limited)
	}
	git("switch", "--detach", baseHash)
	detached := decode[projectLog](t, do(t, s, "GET", url+"/log?graph=true", nil))
	if detached.Branch != "" || detached.Head != baseHash {
		t.Fatalf("detached: %+v", detached)
	}
}
