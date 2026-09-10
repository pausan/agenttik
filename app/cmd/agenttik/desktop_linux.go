//go:build desktop && linux

package main

import (
	"io/fs"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"github.com/pausan/agenttik/web"
)

func configureDesktop(app *options.App) {
	app.Linux = &linux.Options{
		Icon: appIcon(),
		// Wails only picks this default while Linux options are nil, and
		// the webview goes blank on some drivers with acceleration on.
		WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
	}
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
