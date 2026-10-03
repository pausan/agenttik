//go:build desktop && !linux && !darwin

package main

import "github.com/wailsapp/wails/v2/pkg/options"

func configureDesktop(*options.App) {}
