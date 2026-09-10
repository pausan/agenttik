# Cross-platform builds and releases

`.github/workflows/build-and-release.yml` builds the desktop app, including its
embedded UI, on every push and manual dispatch. The build matrix uses native
runners for Linux amd64 and arm64, macOS arm64, and Windows amd64.

The matrix carries the per-platform build flags: the build tags, the Go
`-ldflags`, and `CGO_LDFLAGS`. Both flag entries are only passed when they are
set, so the Go defaults stand everywhere else. macOS needs
`-framework UniformTypeIdentifiers`, because Wails calls `UTType` for the file
dialog filters and only the `wails` CLI adds that framework; a plain
`go build` fails to link without it.

Only a pushed tag that exactly matches `vMAJOR.MINOR.PATCH` creates a GitHub
Release. The release job waits for all matrix builds to succeed, then attaches
`agenttik_<tag>_<platform>_<architecture>` binaries, with `.exe` on Windows.
