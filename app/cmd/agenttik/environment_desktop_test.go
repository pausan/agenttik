//go:build desktop && (darwin || linux)

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent/claudecode"
)

func TestRestoreDesktopPath(t *testing.T) {
	isolateBundledCodexDirs(t)
	shell := filepath.Join(t.TempDir(), "shell")
	writeExecutable(t, shell, "#!/bin/sh\nexport PATH=/shell/bin\n/bin/sh -c \"$2\"\n")
	systemBinDirs = nil
	t.Cleanup(func() { systemBinDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"} })
	for _, test := range []struct {
		term, want string
	}{
		{"", "/original/bin:/shell/bin"},
		{"dumb", "/original/bin:/shell/bin"},
		{"xterm-256color", "/original/bin"},
	} {
		t.Run(test.term, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("TERM", test.term)
			t.Setenv("SHELL", shell)
			t.Setenv("PATH", "/original/bin")
			restoreDesktopPath()
			if got := os.Getenv("PATH"); got != test.want {
				t.Fatalf("PATH = %q, want %q", got, test.want)
			}
		})
	}
}

// A shell that fails, or takes too long, still leaves Claude Code's native
// install findable.
func TestRestoreDesktopPathFindsInstallDirsWithoutShell(t *testing.T) {
	isolateBundledCodexDirs(t)
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "claude"), "#!/bin/sh\n")
	shell := filepath.Join(home, "shell")
	writeExecutable(t, shell, "#!/bin/sh\nexit 1\n")
	systemBinDirs = []string{filepath.Join(home, "missing")}
	t.Cleanup(func() { systemBinDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"} })
	t.Setenv("HOME", home)
	t.Setenv("TERM", "")
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/original/bin")
	restoreDesktopPath()
	if got, want := os.Getenv("PATH"), "/original/bin:"+bin; got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
	if err := claudecode.New().Available(); err != nil {
		t.Fatal(err)
	}
}
