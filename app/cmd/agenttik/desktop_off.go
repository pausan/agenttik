//go:build !desktop

package main

import (
	"net/http"

	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
)

func runDesktop(*server.Server, *runner.Runner, *single.Lock, string) error { return errNoDesktop }

func runRemoteDesktop(http.Handler, string) error { return errNoDesktop }
