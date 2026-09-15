//go:build darwin || linux

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent/claudecode"
	"github.com/pausan/agenttik/app/internal/agent/codex"
)

func TestRestoreShellPathFindsAndRunsCLIs(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "custom node", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "node"), "#!/bin/sh\nprintf 'runtime found'\n")
	for _, name := range []string{"codex", "claude"} {
		writeExecutable(t, filepath.Join(bin, name), "#!/usr/bin/env node\n")
	}
	// Simulate startup files adding a version manager's bin directory. Check
	// that the shell is interactive and a login shell, then run the real probe.
	shell := filepath.Join(home, "shell")
	writeExecutable(t, shell, `#!/bin/sh
[ "$1" = '-ilc' ] || exit 1
[ "$PWD" = "$HOME" ] || exit 2
printf 'Welcome!\n'
export PATH="$HOME/custom node/bin:$PATH"
export AGENTTIK_PATH_TEST_SECRET=from-shell
/bin/sh -c "$2"
printf 'Goodbye!\n'
`)
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AGENTTIK_PATH_TEST_SECRET", "inherited")
	if codex.New().Available() == nil || claudecode.New().Available() == nil {
		t.Fatal("test CLIs must be unavailable before recovering PATH")
	}
	if err := restoreShellPath(context.Background(), shell); err != nil {
		t.Fatal(err)
	}
	if err := codex.New().Available(); err != nil {
		t.Fatal(err)
	}
	if err := claudecode.New().Available(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "claude"} {
		out, err := exec.Command(name).Output()
		if err != nil || string(out) != "runtime found" {
			t.Fatalf("%s: output %q, error %v", name, out, err)
		}
	}
	if got := os.Getenv("AGENTTIK_PATH_TEST_SECRET"); got != "inherited" {
		t.Fatalf("imported unrelated shell environment: %q", got)
	}
}

func TestRestoreShellPathFailurePreservesEnvironment(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"exit", "exit 1"},
		{"banner only", "printf 'hello\\n'"},
		{"empty", `printf '\000agenttik-path\000\n\000agenttik-path\000'`},
		{"incomplete", `printf '\000agenttik-path\000/opt/bin'`},
		{"timeout", "/bin/sleep 30 & wait"},
	} {
		t.Run(test.name, func(t *testing.T) {
			shell := filepath.Join(t.TempDir(), "shell")
			writeExecutable(t, shell, "#!/bin/sh\n"+test.body+"\n")
			t.Setenv("PATH", "/original/bin")
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			if err := restoreShellPath(ctx, shell); err == nil {
				t.Fatal("expected shell lookup failure")
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("shell lookup did not respect its deadline")
			}
			if got := os.Getenv("PATH"); got != "/original/bin" {
				t.Fatalf("PATH changed on failure: %q", got)
			}
		})
	}
}

func TestAppendShellPath(t *testing.T) {
	for _, test := range []struct {
		name, inherited, shell, want string
	}{
		{"priority and duplicates", "/chosen:/usr/bin", "/usr/bin:/custom:/chosen:/custom", "/chosen:/usr/bin:/custom"},
		{"empty inherited", "", "/custom:/usr/bin", "/custom:/usr/bin"},
		{"relative shell entries", "/usr/bin", ":.:relative:/custom:", "/usr/bin:/custom"},
		{"spaces and newlines", "/usr/bin", "/a b:/a\nb", "/usr/bin:/a b:/a\nb"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := appendShellPath(test.inherited, test.shell); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
}
