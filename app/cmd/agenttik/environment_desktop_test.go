//go:build desktop && (darwin || linux)

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreDesktopPath(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	writeExecutable(t, shell, "#!/bin/sh\nexport PATH=/shell/bin\n/bin/sh -c \"$2\"\n")
	for _, test := range []struct {
		term, want string
	}{
		{"", "/original/bin:/shell/bin"},
		{"dumb", "/original/bin:/shell/bin"},
		{"xterm-256color", "/original/bin"},
	} {
		t.Run(test.term, func(t *testing.T) {
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
