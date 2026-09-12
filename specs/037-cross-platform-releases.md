# Cross-platform builds and releases

`.github/workflows/build-and-release.yml` builds the desktop app, including its
embedded UI, on every push, pull request and manual dispatch. The build matrix
uses native runners for Linux amd64 and arm64, macOS arm64, and Windows amd64.

The matrix carries the per-platform build flags: the build tags, extra Go
`-ldflags`, and `CGO_LDFLAGS`. Those last two are only added when the platform
sets them. All matrix builds use `-trimpath` and Go linker flags `-s -w` to omit
the symbol table and debug information. macOS needs
`-framework UniformTypeIdentifiers`, because Wails calls `UTType` for the file
dialog filters and only the `wails` CLI adds that framework; a plain
`go build` fails to link without it.

Linux amd64, Linux arm64 and Windows amd64 binaries are compressed in place
with UPX 5.1.1 (`--best --lzma`) before upload. The workflow downloads the native
UPX release from `upx/upx` and runs `upx --test` on the compressed executable.
macOS arm64 is stripped only: [UPX does not support that target](https://github.com/upx/upx/discussions/931).
Every platform runs the final executable with `--version` and checks its output
before upload. Compression, integrity or version-check failures fail the build.

Only a pushed tag matching `vMAJOR.MINOR.PATCH`, with an optional suffix of
letters, dashes and further dots, creates a GitHub Release. The release job
waits for all matrix builds to succeed, then attaches
`agenttik_<tag>_<platform>_<architecture>` binaries, with `.exe` on Windows.
That job only downloads those artifacts, so it has no checkout for `gh` to read
a git remote from and names the repository in `GH_REPO` instead.
An untagged push names its binaries after the short commit instead.

Every build links in the version the binary reports, on top of whatever
`-ldflags` the matrix already carries; see 044 for what that version is.
