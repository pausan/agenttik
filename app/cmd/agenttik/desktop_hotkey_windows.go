//go:build desktop && windows

package main

import (
	"unsafe"

	"golang.design/x/hotkey"
	"golang.org/x/sys/windows"
)

const (
	altModifier     = hotkey.ModAlt
	commandModifier = hotkey.ModWin
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// appIsForeground reports whether the window the desktop has in front belongs
// to this process.
func appIsForeground() bool {
	front, _, _ := procGetForegroundWindow.Call()
	if front == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(front, uintptr(unsafe.Pointer(&pid)))
	return pid != 0 && pid == windows.GetCurrentProcessId()
}
