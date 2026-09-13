# Desktop tray and global shortcut

Settings → General → Desktop tray offers an opt-in **Close to tray** setting
and a **Show / hide shortcut**, defaulting to `Ctrl+Shift+A` on Windows and
Linux and `Cmd+Shift+A` on macOS. Save and restart the desktop app to apply
either change. The window opens normally at launch.
Closing it hides it and keeps the server and tasks running. The shortcut works
with another application focused. When agenttik is the window in front, the
shortcut hides it to the tray. From anywhere else — behind another
application, minimised, or already in the tray — it shows the window and
brings it to the front. Holding the shortcut acts once.

The tray menu provides **Show / Hide agenttik** and **Quit**. Its Show / Hide
action toggles visibility regardless of which application has focus. Quit
bypasses close-to-tray and runs the normal shutdown. A second launch restores
a hidden window through the existing foreground hook. Disabling Close to tray
and restarting removes the tray and releases the shortcut; closing then exits.
`Ctrl+Q`, or `Cmd+Q` on macOS, is a fixed app-window quit shortcut: it exits
the process from the focused window even when Close to tray is enabled. It is
not registered globally and cannot be chosen as the show/hide shortcut.

## Storage and API

SQLite's singleton `desktop_config` row stores `close_to_tray` (default false)
and `toggle_shortcut`. `GET /api/desktop` adds `available` (desktop shell) and
`error` (native startup failure). `PUT /api/desktop` validates and saves both
fields. Web mode reports unavailable and rejects writes. The UI hides the
controls in web mode. Settings search includes the tray and shortcut fields.

A shortcut contains one or more distinct `Ctrl`, `Cmd`, `Alt`, `Shift`
modifiers and a letter A–Z, digit 0–9, or F1–F12, separated by `+`; the
platform quit chord is reserved for quitting. This desktop shortcut is stored
separately from the browser shortcuts because it must be registered before the
webview opens. Native registration checks availability on startup.
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

The shortcut asks the operating system which window is in front at the moment
it fires, rather than mirroring focus events from the UI, because a webview
reports focus late and misses window manager changes altogether. Linux reads
`_NET_ACTIVE_WINDOW` and its `_NET_WM_PID` over the same X11 connection that
holds the grab; Windows compares the process behind `GetForegroundWindow` with
its own; macOS asks `NSRunningApplication`. A check that cannot answer counts
as not in front, so the shortcut shows the window.

Linux configures JavaScriptCore to use signal 34 before WebKit starts when the
environment has not chosen a value, so its GC signal does not collide with
Go's signal handler. An explicit value is applied through the same API. It
uses the StatusNotifier/AppIndicator tray protocol and a dedicated X11
connection for the global key grab, including Caps Lock/Num Lock variants.
The event reader blocks while idle. Matching release/press timestamps filter
X11's synthetic auto-repeat pairs. Closing the connection releases all grabs
and stops the reader; the known nil-event diagnostic from the XGB dependency is
not shown, while other XGB diagnostics remain visible. Linux requires an X11
session and a tray host (e.g. GNOME's AppIndicator extension); Wayland is
currently unsupported.
Windows and macOS use `golang.design/x/hotkey`. On macOS, stored `Ctrl` tray
chords from earlier versions are registered and displayed with `Cmd`. macOS
requires Accessibility permission for its event tap; registration failure
appears in Settings.

## Verification

Store/API tests cover defaults, persistence, invalid chords, web mode and
startup error reporting. Browser tests cover saving and reopening settings
and hiding the controls in web mode. The isolated Linux test exercises the
default chord, auto-repeat, the reported foreground state, registration
conflict and unregister/re-register:

```sh
AGENTTIK_TEST_HOTKEY=1 xvfb-run -a go test -race -tags 'desktop production webkit2_41' ./app/cmd/agenttik -run TestGlobalToggle -count=1
```

A live Linux smoke check verifies that the shortcut hides the window it has in
front and raises it from behind another application, from minimised and from
the tray, plus close-to-tray, second-launch restore, Ctrl+Q with Close to tray
enabled, and tray Quit. Linux
desktop and Windows cross-builds are checked. macOS runtime behavior
requires verification on a Mac.
