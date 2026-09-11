package claudecode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountStatusReadsThePlanAndNotTheToken(t *testing.T) {
	home := t.TempDir()
	// The real file holds tokens beside these fields. Only the plan is read;
	// nothing here decodes or returns a secret.
	body := `{"claudeAiOauth":{"accessToken":"secret-value","subscriptionType":"team"}}`
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	status := New().AccountStatus(home)
	if !status.SignedIn {
		t.Error("a directory with credentials reads as signed out")
	}
	if status.Detail != "team plan" {
		t.Errorf("detail = %q, want %q", status.Detail, "team plan")
	}
	if strings.Contains(status.Detail, "secret-value") {
		t.Error("the token leaked into the status")
	}
}

func TestAccountStatusOnAnEmptyDirectory(t *testing.T) {
	status := New().AccountStatus(t.TempDir())
	if status.SignedIn {
		t.Error("an empty directory reads as signed in")
	}
}

func TestDefaultHomeFollowsTheEnvironment(t *testing.T) {
	t.Setenv(HomeVar, "/tmp/elsewhere")
	// An override already in this process's environment is what the CLI would
	// use too, so it is what the system account points at.
	if got := New().DefaultHome(); got != "/tmp/elsewhere" {
		t.Errorf("default home = %q, want the overridden one", got)
	}
}

func TestLoginCommandSetsTheAccountDirectory(t *testing.T) {
	cmd := New().LoginCommand("/tmp/work")
	if len(cmd.Env) != 1 || cmd.Env[0] != HomeVar+"=/tmp/work" {
		t.Errorf("env = %v, want the config dir set", cmd.Env)
	}
	if strings.Join(cmd.Args, " ") != Binary+" /login" {
		t.Errorf("args = %v", cmd.Args)
	}
	// The machine's own login needs no override, and must not be given one:
	// that is the account the CLI already uses.
	if env := New().LoginCommand("").Env; len(env) != 0 {
		t.Errorf("the system login carries %v", env)
	}
}
