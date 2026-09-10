# Linux releases

Pushing any Git tag runs `.github/workflows/release-linux.yml`. It builds the
desktop binary natively for Linux amd64 and arm64, including the embedded web
UI. Asset names use the pushed tag: `agenttik_<tag>_linux_<architecture>`.

A tag matching `vMAJOR.MINOR.PATCH`, with optional prerelease or build metadata,
creates a GitHub Release and attaches both binaries. Other tag builds are kept
only as private GitHub Actions artifacts for 14 days.
