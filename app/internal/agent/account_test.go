package agent

import (
	"os"
	"strings"
	"testing"
)

func TestHomeEnvInheritsForTheSystemAccount(t *testing.T) {
	// nil is exec's own "inherit mine unchanged", which is what keeps a
	// single-subscription machine running exactly the command it always did.
	if env := HomeEnv("CLAUDE_CONFIG_DIR", ""); env != nil {
		t.Errorf("the system account produced an environment of its own: %v", env)
	}
}

func TestHomeEnvOverridesOneVariable(t *testing.T) {
	t.Setenv("AGENTTIK_TEST_MARKER", "kept")
	env := HomeEnv("CLAUDE_CONFIG_DIR", "/tmp/work")
	var marker, home bool
	for _, entry := range env {
		if entry == "AGENTTIK_TEST_MARKER=kept" {
			marker = true
		}
		if entry == "CLAUDE_CONFIG_DIR=/tmp/work" {
			home = true
		}
	}
	if !home {
		t.Errorf("account home missing from %d entries", len(env))
	}
	if !marker {
		t.Error("the rest of the environment was dropped")
	}
	if len(env) != len(os.Environ())+1 {
		t.Errorf("environment has %d entries, want the process's %d plus one",
			len(env), len(os.Environ()))
	}
}

// A login command has to be renderable as something a human can run, so the
// pieces are kept separate rather than pre-joined.
func TestLoginCommandCarriesEnvAndArgsApart(t *testing.T) {
	cmd := LoginCommand{Env: []string{"CODEX_HOME=/tmp/a b"}, Args: []string{"codex", "login"}}
	if len(cmd.Args) == 0 || cmd.Args[0] != "codex" {
		t.Fatalf("args = %v", cmd.Args)
	}
	if !strings.HasPrefix(cmd.Env[0], "CODEX_HOME=") {
		t.Errorf("env = %v", cmd.Env)
	}
}
