//go:build desktop && darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void setAppIconPNG(const void *data, int length);
*/
import "C"

import (
	_ "embed"
	"unsafe"
)

// A binary run outside its .app bundle has no icon of its own, so macOS shows
// a terminal in the Dock and the Cmd+Tab switcher. Setting it at runtime
// covers that case and matches the bundle's icon otherwise.
//
//go:embed appicon.png
var appIconPNG []byte

func setAppIcon() {
	C.setAppIconPNG(unsafe.Pointer(&appIconPNG[0]), C.int(len(appIconPNG)))
}
