//go:build desktop && (darwin || linux)

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent/claudecode"
	"github.com/pausan/agenttik/app/internal/agent/codex"
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

func isolateBundledCodexDirs(t *testing.T) {
	t.Helper()
	original := bundledCodexDirs
	bundledCodexDirs = nil
	t.Cleanup(func() { bundledCodexDirs = original })
}

func TestRestoreDesktopPathFindsBundledCodex(t *testing.T) {
	isolateBundledCodexDirs(t)
	home := t.TempDir()
	bin := filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex-cli", "CodexCLI.app", "Contents", "MacOS")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "codex"), "#!/bin/sh\nprintf 'bundled codex'\n")
	bundledCodexDirs = []string{filepath.Join(home, "missing"), bin}
	systemDirs := systemBinDirs
	systemBinDirs = nil
	t.Cleanup(func() { systemBinDirs = systemDirs })
	t.Setenv("HOME", home)
	t.Setenv("TERM", "")
	t.Setenv("SHELL", filepath.Join(home, "missing-shell"))
	t.Setenv("PATH", "/usr/bin:/bin")
	if codex.New().Available() == nil {
		t.Fatal("bundled Codex must be unavailable before restoring PATH")
	}
	restoreDesktopPath()
	if err := codex.New().Available(); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("codex").Output()
	if err != nil || string(out) != "bundled codex" {
		t.Fatalf("Codex output %q, error %v", out, err)
	}
	// An installed CLI wins over the app bundle, including after another probe.
	installed := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(installed, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(installed, "codex"), "#!/bin/sh\n")
	t.Setenv("PATH", installed+":/usr/bin:/bin")
	restoreDesktopPath()
	if got, err := exec.LookPath("codex"); err != nil || got != filepath.Join(installed, "codex") {
		t.Fatalf("selected Codex %q, error %v", got, err)
	}
}
