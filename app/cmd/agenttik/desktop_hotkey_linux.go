//go:build desktop && linux

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/godbus/dbus/v5"
)

func checkTray() error {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return fmt.Errorf("global tray shortcuts require an X11 session; Wayland is not supported")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("system tray unavailable: %w", err)
	}
	defer conn.Close()
	value, err := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").GetProperty("org.kde.StatusNotifierWatcher.IsStatusNotifierHostRegistered")
	if err != nil || value.Value() != true {
		return fmt.Errorf("no system tray host is available; enable your desktop's AppIndicator extension")
	}
	return nil
}

func registerToggle(chord string, toggle func()) (func(), error) {
	xu, err := xgbutil.NewConn()
	if err != nil {
		return nil, fmt.Errorf("global shortcut: %w", err)
	}
	keybind.Initialize(xu)
	chord = strings.NewReplacer("Ctrl", "Control", "Alt", "Mod1", "+", "-").Replace(chord)
	mods, codes, err := keybind.ParseString(xu, chord)
	if err != nil {
		xu.Conn().Close()
		return nil, err
	}
	for _, code := range codes {
		if err := keybind.GrabChecked(xu, xu.RootWin(), mods, code); err != nil {
			xu.Conn().Close()
			return nil, fmt.Errorf("global shortcut is unavailable (it may already be in use): %w", err)
		}
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		down := false
		var released xproto.Timestamp
		matches := func(code xproto.Keycode) bool {
			for _, wanted := range codes {
				if code == wanted {
					return true
				}
			}
			return false
		}
		for {
			event, err := xu.Conn().WaitForEvent()
			if event == nil && err == nil {
				return
			}
			if err != nil {
				continue
			}
			switch e := event.(type) {
			case xproto.KeyPressEvent:
				if !matches(e.Detail) {
					continue
				}
				// X11 auto-repeat emits release/press pairs with the same timestamp.
				if !down && e.Time != released {
					toggle()
				}
				down = true
			case xproto.KeyReleaseEvent:
				if matches(e.Detail) {
					down = false
					released = e.Time
				}
			}
		}
	}()
	return func() {
		xu.Conn().Close()
		<-finished
	}, nil
}
