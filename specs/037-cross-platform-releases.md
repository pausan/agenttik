# Cross-platform builds and releases

`.github/workflows/build-and-release.yml` builds the desktop app, including its
embedded UI, on every push, pull request and manual dispatch. The build matrix
uses native runners for Linux amd64 and arm64, macOS arm64, and Windows amd64.

UI builds use Node 22 and `npm ci` with the committed `web/package-lock.json`.
Keep the lockfile compatible with npm 10, including bundled optional dependencies
for other platforms. After dependency changes, validate it with
`npx --yes npm@10.9.8 ci --prefix web` before running the UI build.

The matrix carries the per-platform build flags: the build tags, extra Go
`-ldflags`, and `CGO_LDFLAGS`. Those last two are only added when the platform
sets them. All matrix builds use `-trimpath` and Go linker flags `-s -w` to omit
the symbol table and debug information. macOS needs
`-framework UniformTypeIdentifiers`, because Wails calls `UTType` for the file
dialog filters and only the `wails` CLI adds that framework; a plain
`go build` fails to link without it.

Linux amd64 and Linux arm64 binaries are compressed in place
with UPX 5.1.1 (`--best --lzma`) before upload. The workflow downloads the native
UPX release from `upx/upx` and runs `upx --test` on the compressed executable.
macOS arm64 is stripped only: [UPX does not support that target](https://github.com/upx/upx/discussions/931).
Every platform runs the final executable with `--version` and checks its output
before upload. Compression, integrity or version-check failures fail the build.

Windows amd64 stays unpacked to reduce antivirus false positives. The Python
3.11+ script `scripts/package-windows.py` creates
`agenttik_<tag>_windows_amd64.zip` containing only the matching `.exe` at its root
and verifies its SHA-256 against the original binary. CI and
`make build-windows-amd64` both produce the ZIP. The standalone, unpacked `.exe`
is also published because existing automatic updaters require that asset.
Windows binaries are unsigned; ZIP packaging does not establish publisher trust
or guarantee that antivirus and SmartScreen warnings disappear.

Only a pushed tag matching `vMAJOR/vMAJOR.MINOR.PATCH`, with an optional suffix of
letters, dashes and further dots, creates a GitHub Release. The release job
waits for all matrix builds to succeed, then attaches
`agenttik_<tag>_<platform>_<architecture>` binaries, with `.exe` on Windows.
The `<tag>` in filenames is the final component, such as `v0.7.1`; the
release title also uses that final component while targeting the full
`v0/v0.7.1` tag. Legacy unprefixed tags are also accepted.
That job only downloads those artifacts, so it has no checkout for `gh` to read
a git remote from and names the repository in `GH_REPO` instead.
An untagged push names its binaries after the short commit instead.

Run `python3 scripts/test-build-version.py` to verify local and CI tag parsing.
Run `python3 scripts/test-package-windows.py` to verify ZIP layout and executable
byte preservation, including rebuilding an existing archive.

Every build links in the version the binary reports, on top of whatever
`-ldflags` the matrix already carries; see 044 for what that version is.

## macOS application bundle

macOS builds retain the standalone executable and also produce `agenttik.app`,
plus `agenttik_<tag>_darwin_arm64.app.zip` and `agenttik_<tag>_darwin_arm64.dmg`
release assets. `make build` on macOS creates `bin/agenttik.app`,
`bin/agenttik_darwin.app.zip` and `bin/agenttik_darwin.dmg`;
`make build-macos-arm64` creates `dist/agenttik.app`,
`dist/agenttik_darwin_arm64.app.zip` and `dist/agenttik_darwin_arm64.dmg`
alongside its executable.

`scripts/package-macos.sh` supplies an Info.plist with stable bundle identifier
`com.pausan.agenttik`, executable, icon and version metadata. Commit builds use
numeric bundle version `0.0.0` and retain the commit in the informational string
and `--version`. The bundle uses free ad-hoc signing (`codesign --sign -`), with
no Apple account or signing secrets. It is not notarized: downloaded copies may
require **System Settings → Privacy & Security → Open Anyway** after the first
launch attempt.

The app icon comes from `scripts/macos/icon.png`, a 1024px render of
`web/public/agenttik.svg` on the macOS icon grid; `sips` and `iconutil` turn it
into every `.icns` size from 16 to 1024 pixels.

The DMG is for manual installs and copies the
[KeePassXC](https://github.com/keepassxreboot/keepassxc) look: a 660×400
window with no toolbar, `agenttik.app` on the left, an `Applications` symlink
on the right, both at 156 points, and a chevron between them on a light
background. It is a compressed (UDZO) HFS+ image made with `hdiutil`, volume
name `agenttik`. No Finder scripting runs at build time, so it works on
headless CI. Instead the image carries committed files from `scripts/macos/`:

- `DS_Store`, copied to `.DS_Store`, holds the window, icon size and positions.
  `make-ds-store.py` writes it (needs the `ds_store` and `mac_alias` Python
  packages). It finds the background through an alias to
  `agenttik:.background:background.tiff`, so the volume name and that path
  must not change.
- `dmg-background.png` and `dmg-background@2x.png` are merged by `tiffutil`
  into `.background/background.tiff` for normal and Retina screens.
  `render-images.mjs` draws them and the icon with the e2e Playwright Chromium.

If another volume named `agenttik` is already mounted, Finder may show the
image without its background; the drag-to-install still works. The ZIP stays
because the updater installs from it.

The packager validates the plist and signature, archives with `ditto`, then
extracts the ZIP and verifies the extracted signature and executable version.
It also verifies the DMG checksum, mounts it read-only, and checks the
`Applications` link, the layout and background files, the app's signature and
its version. Whether Finder shows the layout still requires a live Mac check.
CI runs it on the native macOS runner, along with tray shortcut fallback tests.
Finder launch and tray/Accessibility behavior still require a live Mac check.

The local desktop app checks these release assets daily and can install a
verified newer version on quit; see [070](070-automatic-updates.md).
