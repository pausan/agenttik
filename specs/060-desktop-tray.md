# Desktop tray and global shortcut

Settings → General → Desktop tray offers an opt-in **Close to tray** setting
and a **Show / hide shortcut**, defaulting to `Ctrl+Shift+A`. Save and restart
the desktop app to apply either change. The window opens normally at launch.
Closing it hides it and keeps the server and tasks running. The shortcut works
with another application focused and toggles the window. A minimized window
is restored. Holding the shortcut toggles once.

The tray menu provides **Show / Hide agenttik** and **Quit**. Quit bypasses
close-to-tray and runs the normal shutdown. A second launch restores a hidden
window through the existing foreground hook. Disabling Close to tray and
restarting removes the tray and releases the shortcut; closing then exits.
`Ctrl+Q` is a fixed quit shortcut: it exits the process from the focused window
and, when the window is hidden in the tray, through the native global shortcut.
It cannot be chosen as the show/hide shortcut.

## Storage and API

SQLite's singleton `desktop_config` row stores `close_to_tray` (default false)
and `toggle_shortcut`. `GET /api/desktop` adds `available` (desktop shell) and
`error` (native startup failure). `PUT /api/desktop` validates and saves both
fields. Web mode reports unavailable and rejects writes. The UI hides the
controls in web mode. Settings search includes the tray and shortcut fields.

A shortcut contains one or more distinct `Ctrl`, `Alt`, `Shift` modifiers and
a letter A–Z, digit 0–9, or F1–F12, separated by `+`; `Ctrl+Q` is reserved for
quitting. This desktop shortcut is stored separately from the browser shortcuts
because it must be registered before the webview opens. Native registration
checks availability on startup.
A missing tray host or failed registration leaves close-to-tray inactive and
reports an error in Settings and the process log; closing still exits normally.

## Native integration

Wails retains its event loop. `cardinalby/go-systray` uses its external-loop
entry point on the main OS thread; its macOS delegate has a distinct name and
does not replace Wails' delegate. The tray embeds PNG/ICO exports of
`web/public/agenttik.svg` independently of the UI build. The PNG is an 8-bit
RGBA browser rendering because simpler SVG rasterizers can discard the logo's
filtered gradient marks and leave an apparently blank dark tile. The ICO
contains 16, 24, 32, 48 and 64 px versions for Windows scaling.

Linux uses the StatusNotifier/AppIndicator tray protocol and a dedicated X11
connection for the global key grab, including Caps Lock/Num Lock variants.
The event reader blocks while idle. Matching release/press timestamps filter
X11's synthetic auto-repeat pairs. Closing the connection releases
all grabs and stops the reader. Linux requires an X11 session and a tray host
(e.g. GNOME's AppIndicator extension); Wayland is currently unsupported.
Windows and macOS use `golang.design/x/hotkey`. macOS requires Accessibility
permission for its event tap; registration failure appears in Settings.

## Verification

Store/API tests cover defaults, persistence, invalid chords, web mode and
startup error reporting. Browser tests cover saving and reopening settings
and hiding the controls in web mode. The isolated Linux test exercises the
default chord, auto-repeat, registration conflict and unregister/re-register:

```sh
AGENTTIK_TEST_HOTKEY=1 xvfb-run -a go test -race -tags 'desktop production webkit2_41' ./app/cmd/agenttik -run TestGlobalToggle -count=1
```

A live Linux smoke check verifies shortcut hide/show, close-to-tray,
second-launch restore, Ctrl+Q from the tray, and tray Quit. Linux desktop and
Windows cross-builds are checked. macOS runtime behavior
requires verification on a Mac.
