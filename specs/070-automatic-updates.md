# Automatic desktop updates

The local desktop app checks the public `pausan/agenttik` GitHub releases API
in the background when due, then once every 24 hours while it runs. The last
attempt, offered asset and ignored version live in `updates.json` in the root
data directory and apply across profiles. Failed requests also count as an
attempt. Startup does not wait for the network. Web-only, remote-only and
private instances do not run the checker. Development/commit builds skip it.

The checker examines the most recent 100 published releases and chooses the
highest stable semantic version newer than the current version with an exact
OS/architecture asset match. Drafts and prereleases are excluded. Assets use
`agenttik_<tag-leaf>_<os>_<arch>`, `.exe` on Windows, or `.app.zip` when running from
a macOS application bundle. A positive size up to 512 MiB and a SHA-256 digest
are required. Metadata comes from the
[GitHub releases API](https://docs.github.com/en/rest/releases/releases).

A small notice offers **Ignore this version** and **Update**. Ignore persists
across launches and does not hide later versions. Update downloads over HTTPS
from the project's release assets, verifies size and SHA-256, and stages an
installer copied from the running executable. Downloads stream to disk.
The app remains usable while preparing; errors appear in the notice with a
retry action. The UI polls local status every minute, every second during
preparation, and delays its first request until three seconds after mounting.

## Installation on quit

The helper copies the verified replacement to the installation volume before
reporting readiness. It waits for the current process to exit, then renames
the current installation to a backup and moves the replacement into place.
A failed move restores the backup; if restoration also fails, the error names
the retained backup. A changed installation path cancels replacement. The user
quits normally and reopens the app; updates do not terminate active tasks or
restart a privileged process. Closing to tray is not quitting.

Writable installation folders need no elevation. Protected folders request
polkit/pkexec on Linux, an administrator dialog through osascript on macOS,
or UAC through PowerShell on Windows. Linux without pkexec reports that it
needs polkit or a user-owned installation. Cancelled/failed preparation leaves
a cancellation marker so a delayed helper cannot install an abandoned update.

macOS bundles are replaced as a whole, preserving their signature and metadata.
The ZIP is checked for unsafe paths, symlinks and oversized contents before
extraction with ditto, followed by codesign verification. Standalone executable
installations use the binary asset. The helper checks the archive digest again
and stages its own copy. Completed helper files are cleaned on the next launch;
installation failures are shown there. An interrupted download or a helper
which never starts can leave an `update-*` directory for manual cleanup.

## API and boundaries

Release versions accept `vMAJOR/vMAJOR.MINOR.PATCH` and legacy unprefixed tags;
asset names use the final tag component. Download URLs retain the full tag,
with either literal or percent-encoded slashes.

`GET /api/updates` reads cached status. `POST /api/updates/ignore` and
`POST /api/updates/install` take only `{ "version": "vMAJOR/vX.Y.Z" }`; callers cannot
supply paths or URLs. The local desktop proxy adds a private random token.
Requests without it see empty status and cannot mutate updates. Profile
windows use the root instance's updater. Exposed listeners and remote windows
do not receive the token.

The OS and architecture refer to the installed desktop app. Checks run only
while that app is open; this does not install an operating-system service.
Older releases without GitHub asset digests are not offered. A process killed
between the two installation renames may require recovery from the retained
`.agenttik-update-*` directory beside the installation.

## Validation

Unit tests cover version ordering, platform/bundle selection, invalid metadata,
durable ignore and daily checks, download integrity, rollback, API isolation,
and a real helper subprocess waiting for its parent to exit. Browser tests
cover ignore, ready-on-quit messaging and failure/retry. Windows is cross-built;
macOS signing and native administrator dialogs still require live OS testing.
