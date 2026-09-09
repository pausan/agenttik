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
| [008-project-workspaces.md](008-project-workspaces.md) | Per-project tab strips, tab groups, file diffs |

Status: sidebar ordering, session archiving, collapsible project session lists,
the project-scoped Tree, fuzzy go-to navigation, global conversation closing,
remembered open tabs, disposable empty sessions, closed-session reopening, and
renaming a session in its row are implemented. Earlier Go, web-unit, and
targeted Sessions/Inspector browser runs pass; this prototype update does not
add or run tests.

Written but not yet run: stepping the tab strip with Ctrl+PageUp/PageDown, the
shortcut list under the sidebar, focusing the Sessions and Tree filters when
their pane is shown, a project page handing its tab over to the session started
from it, and the per-project tab strip — draggable tabs in three coloured
groups, Alt+A…H to switch project, the pulsing dot and shortcut letter on a
project row, `File | Diff` on a file tab over `/api/projects/:id/diff`, and a
per-session prompt draft. None of it has been compiled: `make ui`, `go vet` and
a browser check are still to run.

A Claude Code session started from the UI streams into the transcript and
records per-turn token metrics; resume across turns works. Several sessions can
be open at once, each streaming into its own tab. The Codex provider is a stub.
