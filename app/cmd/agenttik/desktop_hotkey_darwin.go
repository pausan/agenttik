//go:build desktop && darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework Carbon
#include <stdbool.h>
#include <stdint.h>

bool appIsFrontmost(void);
void *registerHotKey(uint32_t code, uint32_t modifiers, uint32_t id, int *status);
void unregisterHotKey(void *ref);
*/
import "C"

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.design/x/hotkey"
)

const (
	altModifier     = hotkey.ModOption
	commandModifier = hotkey.ModCmd
)

// eventHotKeyExistsErr is Carbon's answer when another application already
// holds the chord.
const eventHotKeyExistsErr = -9878

// appIsForeground reports whether this application is the active one and
// still has a window on screen, which is what the desktop has in front.
func appIsForeground() bool { return bool(C.appIsFrontmost()) }

// carbonHotkey is a global shortcut registered with Carbon's
// RegisterEventHotKey. golang.design/x/hotkey uses an event tap on macOS,
// which needs Accessibility permission and blocks forever when registered
// from the main thread, where the tray starts. A Carbon hot key needs neither.
// The library's key and modifier values are Carbon's own, so they pass
// straight through.
type carbonHotkey struct {
	id       uint32
	ref      unsafe.Pointer
	down, up chan hotkey.Event
}

var (
	carbonHotkeys sync.Map // id → *carbonHotkey
	carbonNextID  atomic.Uint32
)

func newGlobalHotkey(mods []hotkey.Modifier, key hotkey.Key) (globalHotkey, error) {
	var modifiers uint32
	for _, mod := range mods {
		modifiers |= uint32(mod)
	}
	hk := &carbonHotkey{id: carbonNextID.Add(1), down: make(chan hotkey.Event, 1), up: make(chan hotkey.Event, 1)}
	carbonHotkeys.Store(hk.id, hk)
	var status C.int
	hk.ref = C.registerHotKey(C.uint32_t(key), C.uint32_t(modifiers), C.uint32_t(hk.id), &status)
	if status == 0 {
		return hk, nil
	}
	carbonHotkeys.Delete(hk.id)
	if status == eventHotKeyExistsErr {
		return nil, fmt.Errorf("the shortcut is already used by another application")
	}
	return nil, fmt.Errorf("register shortcut: macOS error %d", int(status))
}

func (hk *carbonHotkey) Keydown() <-chan hotkey.Event { return hk.down }
func (hk *carbonHotkey) Keyup() <-chan hotkey.Event   { return hk.up }

func (hk *carbonHotkey) Unregister() error {
	C.unregisterHotKey(hk.ref)
	carbonHotkeys.Delete(hk.id)
	return nil
}

// hotkeyEvent runs on the main thread, inside the native event loop, so it
// must not block: an event the reader has not taken yet is enough.
//
//export hotkeyEvent
func hotkeyEvent(id C.uint32_t, pressed C.bool) {
	value, ok := carbonHotkeys.Load(uint32(id))
	if !ok {
		return
	}
	hk := value.(*carbonHotkey)
	events := hk.up
	if pressed {
		events = hk.down
	}
	select {
	case events <- hotkey.Event{}:
	default:
	}
}
