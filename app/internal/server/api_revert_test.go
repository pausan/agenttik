package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevertFile(t *testing.T) {
	for _, scenario := range []string{"modified", "staged", "deleted", "added", "untracked", "rename", "unborn", "literal"} {
		t.Run(scenario, func(t *testing.T) {
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
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			git("init")
			git("config", "user.name", "Test")
			git("config", "user.email", "test@example.com")
			name := "file.txt"
			if scenario == "literal" {
				name = "[a]* -> b.txt"
			}
			if scenario != "unborn" {
				write(name, "original\n")
				write("other.txt", "original\n")
				git("add", ".")
				git("commit", "-m", "Initial")
				write("other.txt", "keep\n")
			}
			path := name
			switch scenario {
			case "modified", "literal":
				write(name, "changed\n")
			case "staged":
				write(name, "staged\n")
				git("add", ".")
				write(name, "unstaged\n")
			case "deleted":
				if err := os.Remove(filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			case "added", "untracked", "unborn":
				path = "new.txt"
				write(path, "new\n")
				if scenario != "untracked" {
					git("add", "--", path)
				}
			case "rename":
				git("mv", name, "renamed.txt")
				path = "renamed.txt"
				write(path, "original\nextra\n")
			}
			p, err := st.CreateProject("revert", root)
			if err != nil {
				t.Fatal(err)
			}
			request := func(path string, want int) {
				t.Helper()
				r := do(t, s, "POST", "/api/projects/"+itoa(p.ID)+"/revert", map[string]string{"path": path})
				defer r.Body.Close()
				if r.StatusCode != want {
					t.Fatalf("revert %q: got %d, want %d", path, r.StatusCode, want)
				}
			}
			request("", 400)
			request("../outside", 400)
			request("*", 400)
			request(path, 200)
			for _, f := range parseStatus(git("status", "--porcelain=v1", "-z")) {
				if f.Path != "other.txt" {
					t.Fatalf("change remains: %+v", f)
				}
			}
			if scenario != "unborn" {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(data) != "original\n" {
					t.Fatalf("original file: %q, %v", data, err)
				}
				data, err = os.ReadFile(filepath.Join(root, "other.txt"))
				if err != nil || string(data) != "keep\n" {
					t.Fatalf("other file changed: %q, %v", data, err)
				}
			}
			if path != name {
				if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
					t.Fatalf("new path remains: %v", err)
				}
			}
			request(path, 400)
		})
	}
}
