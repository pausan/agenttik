# README and icon

## Outcome

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
`agenttik` wordmark; both are gone, and the Projects/Sessions/Tree strip starts
at the top of the panel, aligned with the inspector's strip on the other side.
The app is named by its window title and its favicon, which is where a
single-window local tool is already identified.

## Status

Implemented. SVG parsed and visually checked at 256, 80, and 16 pixels; local
documentation links and whitespace checks pass.

The sidebar was checked in a headless browser: no gradient square left in
either `aside`, the tab strip's top edge matches the inspector's to the pixel,
and Projects/Sessions/Tree still switch and still hand focus to the filter on
the keyboard chord. The window icon is compiled but not visually confirmed —
`make build` links it, and this machine has the librsvg loader, but no one has
looked at the running window's titlebar.
