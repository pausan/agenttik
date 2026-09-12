//go:build desktop

package main

import (
	"context"
	_ "embed"
	"log"
	"runtime"
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
					w.toggle()
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

// quit marks the close as intentional before asking Wails to leave. That flag
// is what makes OnBeforeClose bypass close-to-tray for both Ctrl+Q and the
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

func (w *window) toggle() {
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
