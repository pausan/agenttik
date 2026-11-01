//go:build desktop && !linux

package main

// Only Linux windows take the keyboard: Windows would need a low-level hook
// and macOS an event tap.
func grabKeyboard()    {}
func releaseKeyboard() {}
