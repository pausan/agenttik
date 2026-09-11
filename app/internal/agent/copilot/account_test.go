package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Copilot is the odd one out: its token is in the machine's vault, and the
// config directory only records which account to open it with. So the
// directory travels as a flag rather than an environment variable, and every
// command that reaches the account has to carry it.
func TestBuildArgsCarriesTheConfigDir(t *testing.T) {
	args := strings.Join(buildArgs(agent.TurnRequest{
		Prompt: "hi", Model: "gpt-4.1", AccountHome: "/tmp/work"}), " ")
	if !strings.Contains(args, "--config-dir /tmp/work") {
		t.Errorf("args %q missing the account's config dir", args)
	}
}

func TestBuildArgsLeavesTheSystemAccountAlone(t *testing.T) {
	args := strings.Join(buildArgs(agent.TurnRequest{Prompt: "hi", Model: "gpt-4.1"}), " ")
	if strings.Contains(args, "--config-dir") {
		t.Errorf("the machine's own login was given a config dir: %q", args)
	}
}

func TestAccountStatusNamesTheLogin(t *testing.T) {
	home := t.TempDir()
	body := `{"lastLoggedInUser":{"host":"github.com","login":"octocat"}}`
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	status := New().AccountStatus(home)
	if !status.SignedIn || status.Detail != "octocat" {
		t.Errorf("status = %+v, want octocat signed in", status)
	}
	// A config file with no login recorded is a directory nothing has signed
	// into yet.
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if New().AccountStatus(empty).SignedIn {
		t.Error("a config with no login reads as signed in")
	}
}
