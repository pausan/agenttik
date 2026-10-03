//go:build desktop && darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void startNativeApp(const void *icon, int length, int hooks);
*/
import "C"

import (
	_ "embed"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// A binary run outside its .app bundle has no icon of its own, so macOS shows
// a terminal in the Dock and the Cmd+Tab switcher. Setting it at runtime
// covers that case and matches the bundle's icon otherwise.
//
//go:embed appicon.png
var appIconPNG []byte

// macWindow is the window the Dock and the Quit item act on. It is set once,
// before the hooks that read it are installed.
var macWindow *window

// Without a main menu, macOS has nowhere to send Cmd+C, Cmd+V, Cmd+A or
// Cmd+Z, so text fields lose them. The web view sees every key first, so the
// app's own shortcuts still win over the menu's.
func configureDesktop(app *options.App) {
	app.Menu = menu.NewMenuFromItems(menu.AppMenu(), menu.EditMenu(), menu.WindowMenu())
	app.Mac = &mac.Options{
		// The UI has only a dark theme; this keeps the title bar and native
		// dialogs dark too.
		Appearance: mac.NSAppearanceNameDarkAqua,
		About:      &mac.AboutInfo{Title: "agenttik", Message: "Version " + version, Icon: appIconPNG},
	}
}

// startNative runs once the window exists. A nil window, as in a remote
// window, gets the icon only: with no tray, Wails' own quit and reopen suffice.
func startNative(w *window) {
	hooks := 0
	if w != nil {
		macWindow = w
		hooks = 1
	}
	C.startNativeApp(unsafe.Pointer(&appIconPNG[0]), C.int(len(appIconPNG)), C.int(hooks))
}

// Both run on the main thread, while Wails' calls queue onto it, so they hand
// off to a goroutine rather than wait.

//export appReopen
func appReopen() { go macWindow.present() }

//export appQuit
func appQuit() { go macWindow.quit() }
