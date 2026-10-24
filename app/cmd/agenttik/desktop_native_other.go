//go:build desktop && !darwin && !linux

package main

// Windows needs no additional hooks once the window exists.
func startNative(*window) {}
