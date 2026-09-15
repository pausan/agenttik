package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneRemote(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{"https://github.com/openai/codex", "git@github.com:openai/codex.git", false},
		{"https://github.com/pausan/agenttik-helloworld.git", "https://github.com/pausan/agenttik-helloworld.git", false},
		{"https://github.com/openai/codex/tree/main", "git@github.com:openai/codex.git", false},
		{"https://gitlab.com/group/subgroup/project/-/tree/main", "git@gitlab.com:group/subgroup/project.git", false},
		{"https://example.com/group/project.git", "https://example.com/group/project.git", false},
		{"git@github.com:openai/codex.git", "git@github.com:openai/codex.git", false},
		{"https://github.com/openai", "", true},
		{"ext::sh -c echo", "", true},
		{"--upload-pack=echo", "", true},
		{"relative/repository", "", true},
	}
	for _, tc := range cases {
		got, err := cloneRemote(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("cloneRemote(%q) succeeded, want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("cloneRemote(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestCloneLocalDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := cloneRemote(" " + dir + string(filepath.Separator) + " ")
	if err != nil || got != dir {
		t.Fatalf("cloneRemote(local) = %q, %v; want %q", got, err, dir)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("not a repository"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(dir, "missing")} {
		if _, err := cloneRemote(path); err == nil {
			t.Errorf("cloneRemote(%q) succeeded", path)
		}
	}
}
