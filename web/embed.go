// Package web holds the static UI, compiled into the binary.
//
// The UI is a Vite build (see web/ui), so web/dist is generated rather than
// checked in: `make ui` fills it. A binary built without that step still
// compiles — the directory always holds .gitkeep — and reports the UI as
// missing instead of serving a blank page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Assets is the UI rooted at dist/.
func Assets() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees this exists
	}
	return sub
}

// Built reports whether a compiled UI is embedded in this binary.
func Built() bool {
	_, err := fs.Stat(files, "dist/index.html")
	return err == nil
}
