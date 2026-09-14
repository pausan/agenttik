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
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/app/internal/traypulse"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// These are raster exports of web/public/agenttik.svg, embedded independently
// of the UI build so the native tray always has an icon.
//
//go:embed tray.png
var trayPNG []byte

//go:embed tray.ico
var trayICO []byte

// pulseFrames is how many steps the rising half of the working pulse has.
// Folded back on itself it makes a cycle of twelve, which at traypulse.Step is
// the 1.8s breath.
const pulseFrames = 7

// icoSizes are the sizes the Windows pulse frames carry. The resting icon
// keeps the committed ICO's fuller set; the frames only have to cover what a
// notification area actually asks for, and each one is built while the app
// runs.
var icoSizes = []int{16, 32, 64}

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

// restingIcon is the icon shown whenever nothing is running: the ICO on
// Windows, which is the only format its notification area reads, and the PNG
// everywhere else.
func restingIcon() []byte {
	if runtime.GOOS == "windows" {
		return trayICO
	}
	return trayPNG
}

// workingFrames renders the pulse from the tray icon. A failure here is worth
// a line in the log and nothing more: the tray still starts, and its icon
// stays as still as it was before.
func workingFrames() [][]byte {
	frames, err := traypulse.Frames(trayPNG, pulseFrames)
	if err != nil {
		log.Printf("desktop tray: %v", err)
		return nil
	}
	if runtime.GOOS != "windows" {
		return frames
	}
	for i, frame := range frames {
		ico, err := traypulse.ICO(frame, icoSizes...)
		if err != nil {
			log.Printf("desktop tray: %v", err)
			return nil
		}
		frames[i] = ico
	}
	return frames
}

func (w *window) startTray(config store.DesktopConfig, turns *runner.Runner) func() {
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
	// The pulse is rendered before the tray exists, so a bad icon is found
	// and reported at startup rather than the first time a turn runs.
	pulse := traypulse.New(systray.SetIcon, restingIcon(), workingFrames())
	turns.OnBusy(pulse.SetBusy)

	done := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(1)
	start, end := systray.RunWithExternalLoop(func() {
		defer ready.Done()
		systray.SetIcon(restingIcon())
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

	// The pulse owns the icon from here, and is the only thing that touches
	// it. Shutdown waits for it, so the last icon the tray is handed is the
	// resting one rather than a frame left mid-breath.
	pulsing := make(chan struct{})
	go func() {
		defer close(pulsing)
		pulse.Run(done)
	}()

	return func() {
		stopToggle()
		ready.Wait()
		close(done)
		<-pulsing
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
