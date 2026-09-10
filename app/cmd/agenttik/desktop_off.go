//go:build !desktop

package main

import (
	"github.com/pausan/agenttik/app/internal/server"
	"github.com/pausan/agenttik/app/internal/single"
)

func runDesktop(*server.Server, *single.Lock, string) error { return errNoDesktop }
