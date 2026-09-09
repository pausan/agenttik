//go:build !desktop

package main

import "github.com/pausan/agenttik/app/internal/server"

func runDesktop(*server.Server) error { return errNoDesktop }
