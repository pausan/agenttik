//go:build desktop && !darwin

package main

// Only a macOS binary can run without the icon its package gives it.
func setAppIcon() {}
