# macOS integration

`app/cmd/agenttik/desktop_darwin.go` and `.m` hold what only macOS needs.

**Menu bar.** The window has App, Edit and Window menus. Without a main menu,
macOS has nowhere to send Cmd+C, Cmd+V, Cmd+X, Cmd+A or Cmd+Z, so text fields
lose them. The web view sees every key before the menu does, so the app's own
shortcuts, including Cmd+Q and Cmd+W, still win.

**Appearance and About.** The UI is dark only, so the window uses the
`DarkAqua` appearance; the title bar and native dialogs match it. App ›
About agenttik shows the icon and the build's version.

**Icon.** The binary embeds `appicon.png` and sets it as the application icon
once the window exists, so the Dock and Cmd+Tab show it even when the binary
runs outside its `.app` bundle.

**Dock and Quit.** Once the local window is open, two methods on Wails' app
delegate are replaced:

- `applicationShouldHandleReopen:` raises the window, so clicking the Dock
  icon brings back a window hidden to the tray.
- `applicationShouldTerminate:` runs the same quit as Cmd+Q. Wails' own
  version goes through the close hook, which would hide to the tray, so Quit
  from the Dock, the App menu or a logout would not quit. The App menu's Quit
  item is pointed at `terminate:` so it takes this path too.

Both hand off to a goroutine, since they run on the main thread that Wails'
window calls queue onto. Remote windows have no tray and keep Wails' default
quit; they get the icon only.

Unit tests cannot reach these; a live check on a Mac covers copy/paste in the
prompt, About, Dock click from the tray, and Dock and menu Quit with Close to
tray enabled.
