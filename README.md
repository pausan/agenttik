<img src="web/public/agenttik.svg" alt="agenttik icon" width="80" height="80">

# agenttik

A local workspace for running AI coding sessions across multiple projects.
Use Claude Code, Codex and GitHub Copilot through their official CLIs, with your existing login,
in one desktop window or browser tab. Desktop builds support Linux, macOS ARM64,
and Windows x64.

## Main features

- **Project workspaces:** organize sessions and file tabs; archive and restore conversations.
- **Orchestrator:** enable a separate pinned project in Settings to inspect work and manage tasks across projects, with editable instructions and a reset to the built-in prompt.
- **Live sessions:** stream replies, resume conversations, and queue prompts per project.
- **Model controls:** choose models, effort, permissions, and favorite combinations.
- **Code tools:** browse and edit files, preview Markdown/HTML, and inspect Git diffs and commits.
- **Usage tracking:** view tokens, context usage, subscription allowance, and costs where reported.
- **Local state:** SQLite history, remembered tabs, customizable shortcuts, and color themes.

Claude Code is working; Codex and GitHub Copilot are implemented, with live-turn validation still pending.

## Quick start

Use Go 1.25+, Node.js 22.12+ with npm, and a logged-in `claude`, `codex` or `copilot` CLI on `PATH`.

```sh
make run-web
```

Open [localhost:7717](http://127.0.0.1:7717), add a project folder, and start a session.
Or add one from the terminal with `agenttik --init` in the folder you want —
`agenttik --init path/to/repo` names another — which works whether or not the
app is already open, and shows up in an open window straight away.
`--addr` and `--data-dir` change where it listens and where it keeps its
database, `--web` skips the desktop window, and `--help` and `--version` say
what this build is.
For the desktop app, install the native build dependencies (`make deps` on
Debian/Ubuntu), then run `make run`. `make build-windows-amd64` cross-compiles
the Windows x64 binary; build the macOS ARM64 target on macOS with
`make build-macos-arm64`. macOS builds also create an ad-hoc-signed
`agenttik.app` and a ZIP (`bin/` for `make build`, `dist/` for the ARM64 target).
Extract the release ZIP and move the app to Applications. No Apple developer
account is needed to build it. Downloaded builds are not notarized; after a
blocked launch, use System Settings → Privacy & Security → Open Anyway.

On macOS, Close to tray works even without Accessibility permission. The global
show/hide shortcut needs that permission; grant it in System Settings → Privacy
& Security → Accessibility, then restart agenttik. Shortcut failures appear in
agenttik's desktop settings while the tray menu remains usable.

**No authentication:** anyone who can reach agenttik's address has full control of it. It binds to
loopback only by default; the desktop app's Settings › Server can expose it to the network instead,
which is a trust decision — only do that on a network you trust.

Built with Go, SQLite, Vue 3, Nuxt UI, and Wails. See [specs](specs/index.md)
for architecture and implementation details.

## License

[MIT](LICENSE).
