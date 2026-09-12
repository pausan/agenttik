package server

import "testing"

func TestCloneRemote(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{"https://github.com/openai/codex", "git@github.com:openai/codex.git", false},
		{"https://github.com/openai/codex/tree/main", "git@github.com:openai/codex.git", false},
		{"https://gitlab.com/group/subgroup/project/-/tree/main", "git@gitlab.com:group/subgroup/project.git", false},
		{"https://example.com/group/project.git", "https://example.com/group/project.git", false},
		{"git@github.com:openai/codex.git", "git@github.com:openai/codex.git", false},
		{"https://github.com/openai", "", true},
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
