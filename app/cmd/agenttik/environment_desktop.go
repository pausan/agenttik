//go:build desktop && (darwin || linux)

package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"time"
)

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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := restoreShellPath(ctx, shell); err != nil {
		log.Printf("desktop CLI search path: %v", err)
	}
}
