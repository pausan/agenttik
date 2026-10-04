//go:build desktop && (darwin || linux)

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// systemBinDirs are where Homebrew and most installers put CLIs outside the
// home directory. A variable so tests can leave the machine's own out.
var systemBinDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}

func restoreDesktopPath() {
	// Terminal launches already carry the user's chosen environment, including
	// deliberately restricted paths. Desktop launchers usually have no TERM.
	if term := os.Getenv("TERM"); term != "" && term != "dumb" {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	// Login shells with version managers and plugin frameworks take seconds
	// to start, longer on a cold boot.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := restoreShellPath(ctx, shell); err != nil {
		log.Printf("desktop CLI search path: %v", err)
	}
	// A slow or broken shell must not hide CLIs installed in the usual places.
	os.Setenv("PATH", appendShellPath(os.Getenv("PATH"), strings.Join(installDirs(), string(os.PathListSeparator))))
}

// installDirs lists the usual CLI directories that exist on this machine:
// Claude Code's native installer uses ~/.local/bin, its older local install
// ~/.claude/local.
func installDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"))
	}
	dirs = append(dirs, systemBinDirs...)
	found := dirs[:0]
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			found = append(found, dir)
		}
	}
	return found
}
