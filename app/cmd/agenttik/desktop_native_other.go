//go:build desktop && !darwin

package main

// Only macOS needs native hooks once the window exists.
func startNative(*window) {}
