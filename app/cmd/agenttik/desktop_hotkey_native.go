//go:build desktop && (windows || darwin)

package main

import (
	"fmt"
	"strings"

	"golang.design/x/hotkey"
)

func checkTray() error { return nil }

func registerToggle(chord string, toggle func(foreground bool)) (func(), error) {
	parts := strings.Split(chord, "+")
	var mods []hotkey.Modifier
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Ctrl":
			mods = append(mods, hotkey.ModCtrl)
		case "Shift":
			mods = append(mods, hotkey.ModShift)
		case "Alt":
			mods = append(mods, altModifier)
		}
	}
	// Native key codes are not contiguous on macOS.
	keys := []hotkey.Key{
		hotkey.KeyA, hotkey.KeyB, hotkey.KeyC, hotkey.KeyD, hotkey.KeyE, hotkey.KeyF, hotkey.KeyG, hotkey.KeyH, hotkey.KeyI, hotkey.KeyJ, hotkey.KeyK, hotkey.KeyL, hotkey.KeyM, hotkey.KeyN, hotkey.KeyO, hotkey.KeyP, hotkey.KeyQ, hotkey.KeyR, hotkey.KeyS, hotkey.KeyT, hotkey.KeyU, hotkey.KeyV, hotkey.KeyW, hotkey.KeyX, hotkey.KeyY, hotkey.KeyZ,
		hotkey.Key0, hotkey.Key1, hotkey.Key2, hotkey.Key3, hotkey.Key4, hotkey.Key5, hotkey.Key6, hotkey.Key7, hotkey.Key8, hotkey.Key9,
		hotkey.KeyF1, hotkey.KeyF2, hotkey.KeyF3, hotkey.KeyF4, hotkey.KeyF5, hotkey.KeyF6, hotkey.KeyF7, hotkey.KeyF8, hotkey.KeyF9, hotkey.KeyF10, hotkey.KeyF11, hotkey.KeyF12,
	}
	names := strings.Split("A B C D E F G H I J K L M N O P Q R S T U V W X Y Z 0 1 2 3 4 5 6 7 8 9 F1 F2 F3 F4 F5 F6 F7 F8 F9 F10 F11 F12", " ")
	for i, name := range names {
		if name != parts[len(parts)-1] {
			continue
		}
		hk := hotkey.New(mods, keys[i])
		if err := hk.Register(); err != nil {
			return nil, err
		}
		done := make(chan struct{})
		go func() {
			down := false
			for {
				select {
				case <-hk.Keydown():
					if !down {
						down = true
						toggle(appIsForeground())
					}
				case <-hk.Keyup():
					down = false
				case <-done:
					return
				}
			}
		}()
		return func() { hk.Unregister(); close(done) }, nil
	}
	return nil, fmt.Errorf("invalid shortcut: %s", chord)
}
