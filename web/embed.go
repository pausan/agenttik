// Package web holds the static UI, compiled into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var files embed.FS

// Assets is the UI rooted at static/.
func Assets() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // the embed directive guarantees this exists
	}
	return sub
}
