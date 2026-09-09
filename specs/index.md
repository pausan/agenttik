# Specs index

| Doc | What it covers |
|-----|----------------|
| [001-architecture.md](001-architecture.md) | Process model, packages, request flow |
| [002-data-model.md](002-data-model.md) | SQLite schema, metrics, storage location |
| [003-providers.md](003-providers.md) | Provider interface, CLI invocation, event mapping |
| [004-ui.md](004-ui.md) | Layout, panels, HTTP API |
| [005-testing.md](005-testing.md) | The three test layers and how they are isolated |
| [006-sidebar-ordering.md](006-sidebar-ordering.md) | Sidebar ordering, archiving, and Tree placement |
| [007-session-closing.md](007-session-closing.md) | Closing disposable sessions and reopening tabs |

Status: sidebar ordering, session archiving, collapsible project session lists,
the project-scoped Tree, fuzzy go-to navigation, global conversation closing,
remembered open tabs, disposable empty sessions, and closed-session reopening
are implemented. Earlier Go, web-unit, and targeted Sessions/Inspector browser
runs pass; this prototype update was checked with a production UI build and does
not add or run tests. A Claude Code session started from the UI streams into the
transcript and records per-turn token metrics; resume across turns works.
Several sessions can be open at once, each streaming into its own tab. The
Codex provider is a stub.
