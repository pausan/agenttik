package vpx

import "embed"

// The library scripts/build-vpx.sh writes for this platform, if it was built.
//
//go:embed all:lib/windows_amd64
var embedded embed.FS

const (
	libDir  = "lib/windows_amd64"
	libName = "agenttikvpx.dll"
)
