package codex

import (
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestBuildArgs(t *testing.T) {
	got := strings.Join(buildArgs(agent.TurnRequest{
		WorkDir: "/tmp/p", Model: "gpt-5-codex", Effort: "high",
		Permission: agent.PermissionWorkspace}), " ")
	for _, want := range []string{"exec", "--json", "--cd /tmp/p",
		"--model gpt-5-codex", "model_reasoning_effort=high", "--sandbox workspace-write"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
}

func TestBuildArgsResume(t *testing.T) {
	got := strings.Join(buildArgs(agent.TurnRequest{ProviderSessionID: "s1", WorkDir: "/tmp/p"}), " ")
	if !strings.HasPrefix(got, "exec resume s1") {
		t.Errorf("args %q should start with 'exec resume s1'", got)
	}
}

func TestSandboxMapping(t *testing.T) {
	cases := map[agent.Permission]string{
		agent.PermissionPlan:      "read-only",
		agent.PermissionWorkspace: "workspace-write",
		agent.PermissionFull:      "danger-full-access",
	}
	for perm, want := range cases {
		if got := sandboxFlag(perm); got != want {
			t.Errorf("permission %q -> %q, want %q", perm, got, want)
		}
	}
}
