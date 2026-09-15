//go:build !desktop

package main

import (
	"github.com/pausan/agenttik/app/internal/remote"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
)

func runDesktop(*server.Server, *runner.Runner, *single.Lock, string) error { return errNoDesktop }

func runRemoteDesktop(*remote.Client, string) error { return errNoDesktop }
