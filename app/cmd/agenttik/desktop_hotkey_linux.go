//go:build desktop && linux

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/godbus/dbus/v5"
)

var quietXGBClosedEventLog sync.Once

// xgb reports a closed event channel as an invalid nil event before returning
// the documented (nil, nil) result from WaitForEvent. Keep real XGB messages,
// but drop that one expected shutdown diagnostic.
type xgbLogWriter struct {
	dst io.Writer
}

func (w xgbLogWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Invalid event/error type: <nil>")) {
		return len(p), nil
	}
	return w.dst.Write(p)
}

func suppressClosedXGBEventLog() {
	quietXGBClosedEventLog.Do(func() {
		xgb.Logger.SetOutput(xgbLogWriter{dst: xgb.Logger.Writer()})
	})
}

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

// appIsForeground reports whether the window the desktop has in front belongs
// to this process. The window manager keeps _NET_ACTIVE_WINDOW current and the
// key grab does not disturb it, so this is what the user sees at the moment
// the shortcut fires. Anything it cannot read counts as not in front, which
// leaves the shortcut showing the window rather than hiding it.
func appIsForeground(xu *xgbutil.XUtil) bool {
	active, err := ewmh.ActiveWindowGet(xu)
	if err != nil || active == 0 {
		return false
	}
	pid, err := ewmh.WmPidGet(xu, active)
	return err == nil && int(pid) == os.Getpid()
}

func registerToggle(chord string, toggle func(foreground bool)) (func(), error) {
	suppressClosedXGBEventLog()
	xu, err := xgbutil.NewConn()
	if err != nil {
		return nil, fmt.Errorf("global shortcut: %w", err)
	}
	keybind.Initialize(xu)
	chord = strings.NewReplacer("Ctrl", "Control", "Cmd", "Mod4", "Alt", "Mod1", "+", "-").Replace(chord)
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
					toggle(appIsForeground(xu))
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
