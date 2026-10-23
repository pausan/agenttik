//go:build !linux

package screen

import "runtime"

// Open is where macOS and Windows capture will go; until then there is no
// screen to share.
func Open() (Display, error) {
	return nil, Unavailable("screen sharing is not supported on " + runtime.GOOS + " yet")
}
