//go:build desktop

package main

import (
	"context"
	_ "embed"
	"log"
	"runtime"
	"strings"
	"sync"

	"github.com/cardinalby/go-systray"
	"github.com/pausan/agenttik/app/internal/store"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// These are raster exports of web/public/agenttik.svg, embedded independently
// of the UI build so the native tray always has an icon.
//
//go:embed tray.png
var trayPNG []byte

//go:embed tray.ico
var trayICO []byte

func (w *window) trayStatus() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.trayError
}

func (w *window) trayFailed(err error) {
	log.Printf("desktop tray: %v", err)
	w.mu.Lock()
	w.trayError = err.Error()
	w.mu.Unlock()
}

func (w *window) startTray(config store.DesktopConfig) func() {
	if !config.CloseToTray {
		return func() {}
	}
	config.ToggleShortcut = desktopShortcutForOS(config.ToggleShortcut, runtime.GOOS)
	if err := checkTray(); err != nil {
		w.trayFailed(err)
		return func() {}
	}
	stopToggle, err := registerToggle(config.ToggleShortcut, w.toggle)
	if err != nil {
		w.trayFailed(err)
		return func() {}
	}
	done := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(1)
	start, end := systray.RunWithExternalLoop(func() {
		defer ready.Done()
		icon := trayPNG
		if runtime.GOOS == "windows" {
			icon = trayICO
		}
		systray.SetIcon(icon)
		systray.SetTooltip("agenttik")
		toggle := systray.AddMenuItem("Show / Hide agenttik", config.ToggleShortcut)
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop agenttik")
		w.mu.Lock()
		w.trayReady = true
		w.mu.Unlock()
		go func() {
			for {
				select {
				case <-toggle.ClickedCh:
					w.toggleVisibility()
				case <-quit.ClickedCh:
					w.quit()
				case <-done:
					return
				}
			}
		}()
	}, func() {})
	// Called on the main OS thread, before Wails takes over its native loop.
	start()
	return func() {
		stopToggle()
		ready.Wait()
		close(done)
		end()
	}
}

func desktopShortcutForOS(chord, goos string) string {
	if goos != "darwin" {
		return chord
	}
	parts := strings.Split(chord, "+")
	for i, part := range parts[:len(parts)-1] {
		if part == "Ctrl" {
			parts[i] = "Cmd"
		}
	}
	return strings.Join(parts, "+")
}

// quit marks the close as intentional before asking Wails to leave. That flag
// is what makes OnBeforeClose bypass close-to-tray for both the quit chord and the
// tray menu.
func (w *window) quit() {
	w.mu.Lock()
	if w.quitting || w.ctx == nil {
		w.mu.Unlock()
		return
	}
	w.quitting = true
	ctx := w.ctx
	w.mu.Unlock()
	wruntime.Quit(ctx)
}

// toggle answers the global shortcut. The window goes to the tray only when it
// is already the one in front; from anywhere else — behind another
// application, minimised, or in the tray — the shortcut brings it forward.
// Unminimise is what raises and focuses it, and it is harmless on a window
// that is neither hidden nor minimised.
func (w *window) toggle(foreground bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx == nil || !w.trayReady || w.quitting {
		return
	}
	if foreground {
		wruntime.WindowHide(w.ctx)
		w.hidden = true
		return
	}
	wruntime.WindowShow(w.ctx)
	wruntime.WindowUnminimise(w.ctx)
	w.hidden = false
}

// The tray action remains a visibility toggle. Opening the tray moves focus
// away from the app, so applying the shortcut's rule here would make an
// already visible window come forward instead of hiding it.
func (w *window) toggleVisibility() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx == nil || !w.trayReady || w.quitting {
		return
	}
	if w.hidden || wruntime.WindowIsMinimised(w.ctx) {
		wruntime.WindowShow(w.ctx)
		wruntime.WindowUnminimise(w.ctx)
		w.hidden = false
	} else {
		wruntime.WindowHide(w.ctx)
		w.hidden = true
	}
}

func (w *window) beforeClose(ctx context.Context) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.trayReady || w.quitting {
		return false
	}
	wruntime.WindowHide(ctx)
	w.hidden = true
	return true
}
