//go:build !((linux && (amd64 || arm64)) || (windows && amd64) || (darwin && (amd64 || arm64)))

package vpx

import "embed"

// No library is built for this platform.
var embedded embed.FS

const (
	libDir  = "lib/none"
	libName = "none"
)
