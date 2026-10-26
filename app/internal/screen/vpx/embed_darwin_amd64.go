package vpx

import "embed"

// The library scripts/build-vpx.sh writes for this platform, if it was built.
//
//go:embed all:lib/darwin_amd64
var embedded embed.FS

const (
	libDir  = "lib/darwin_amd64"
	libName = "libagenttikvpx.dylib"
)
