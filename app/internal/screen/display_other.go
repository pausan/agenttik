//go:build !linux && !darwin && !windows

package screen

import "runtime"

// Open reports that this system has no screen capture.
func Open() (Display, error) {
	return nil, Unavailable("screen sharing is not supported on " + runtime.GOOS)
}
