//go:build desktop && linux

package main

/*
#cgo !webkit2_41 pkg-config: javascriptcoregtk-4.0
#cgo webkit2_41 pkg-config: javascriptcoregtk-4.1
#include <stdbool.h>

bool JSConfigureSignalForGC(int);
*/
import "C"

import (
	"io/fs"
	"os"
	"strconv"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"github.com/pausan/agenttik/web"
)

const jscSignalForGC = 34

func configureDesktop(app *options.App) {
	configureJSCSignalForGC()
	app.Linux = &linux.Options{
		Icon: appIcon(),
		// Wails only picks this default while Linux options are nil, and
		// the webview goes blank on some drivers with acceleration on.
		WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
	}
}

func jscSignalForGCValue() int {
	if value, err := strconv.Atoi(os.Getenv("JSC_SIGNAL_FOR_GC")); err == nil && value > 0 {
		return value
	}
	return jscSignalForGC
}

func configureJSCSignalForGC() {
	// Go owns the usual JSC GC signal (SIGUSR1, 10). Signal 34 is not
	// installed by the Go runtime and is accepted by JavaScriptCore as its
	// thread suspend/resume signal. Configure JSC directly before WebKit
	// starts; WebKitGTK logs JSC_SIGNAL_FOR_GC as an invalid option when it is
	// left in the environment.
	C.JSConfigureSignalForGC(C.int(jscSignalForGCValue()))
	_ = os.Unsetenv("JSC_SIGNAL_FOR_GC")
}

// appIcon is the logo the browser tab already shows, taken from the embedded
// UI so there is one copy of it. GTK draws it through gdk-pixbuf, which reads
// SVG only with the loader librsvg installs; without it, or without a built
// UI, the window keeps the toolkit default.
func appIcon() []byte {
	svg, err := fs.ReadFile(web.Assets(), "agenttik.svg")
	if err != nil {
		return nil
	}
	return svg
}
