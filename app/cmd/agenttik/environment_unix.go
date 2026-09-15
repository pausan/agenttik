//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/process"
)

// restoreShellPath runs before providers or background jobs start, so discovery
// and every CLI child inherit the same PATH (including interpreters like node).
func restoreShellPath(ctx context.Context, shell string) error {
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("shell must be an absolute path")
	}
	// Interactive login mode includes paths from both login files and rc files
	// used by version managers. printenv also handles fish's list-valued PATH.
	// NUL markers keep banners and logout messages out of the result.
	const marker = "\x00agenttik-path\x00"
	cmd := exec.CommandContext(ctx, shell, "-ilc", `printf '\000agenttik-path\000'; /usr/bin/printenv PATH; printf '\000agenttik-path\000'`)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	process.Configure(cmd)
	cmd.Cancel = func() error { return process.Kill(cmd.Process) }
	// A startup script may leave a descendant holding stdout open.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("read shell PATH: %w", err)
	}
	_, rest, ok := bytes.Cut(out, []byte(marker))
	if !ok {
		return fmt.Errorf("shell did not report PATH")
	}
	path, _, ok := bytes.Cut(rest, []byte(marker))
	if !ok || len(path) == 0 || path[len(path)-1] != '\n' {
		return fmt.Errorf("shell reported an incomplete PATH")
	}
	path = path[:len(path)-1] // printenv's newline, not whitespace in a directory
	if len(path) == 0 {
		return fmt.Errorf("shell reported an empty PATH")
	}
	return os.Setenv("PATH", appendShellPath(os.Getenv("PATH"), string(path)))
}

func appendShellPath(inherited, shell string) string {
	paths := filepath.SplitList(inherited)
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		seen[path] = true
	}
	for _, path := range filepath.SplitList(shell) {
		// Do not add current-directory lookups from shell startup files.
		if filepath.IsAbs(path) && !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	return strings.Join(paths, string(os.PathListSeparator))
}
