# Cross-platform builds and releases

`.github/workflows/build-and-release.yml` builds the desktop app, including its
embedded UI, on every push and manual dispatch. The build matrix uses native
runners for Linux amd64 and arm64, macOS amd64, and Windows amd64.

Only a pushed tag that exactly matches `vMAJOR.MINOR.PATCH` creates a GitHub
Release. The release job waits for all matrix builds to succeed, then attaches
`agenttik_<tag>_<platform>_<architecture>` binaries, with `.exe` on Windows.
