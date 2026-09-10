# README and icon

The README introduces the local, multi-project coding workspace, lists its main
features, and keeps a short quick start. The existing MIT license is linked once,
at the end. Codex's pending live-turn validation and the local-only server warning
remain explicit.

`web/public/agenttik.svg` is the one logo: the README, the browser favicon and
the desktop window all read the same file. It has an accessible title and
description, with no fonts, scripts, external assets, or new dependencies. Vite
copies the public asset into the embedded UI.

The desktop window takes its icon from that embed rather than from a second
copy on disk: `appIcon` in `desktop.go` reads `agenttik.svg` out of
`web.Assets()`. GTK draws it through gdk-pixbuf, which reads SVG only where
librsvg installed the loader; without it, or in a binary built without `make
ui`, the read or the decode fails quietly and the window keeps the toolkit
default. Passing `linux.Options` at all means the GPU policy has to be set
explicitly — Wails only defaults it to `Never` while those options are nil, and
the webview goes blank on some drivers with acceleration on.

The sidebar carries no logo. It used to open with a gradient square and an
`agenttik` wordmark; both are gone, and the Projects/Tree strip starts
at the top of the panel, aligned with the inspector's strip on the other side.
The app is named by its window title and its favicon, which is where a
single-window local tool is already identified.

The window icon is compiled in but has never been looked at: `make build` links
it and this machine has the librsvg loader, but nobody has checked the running
window's titlebar.
