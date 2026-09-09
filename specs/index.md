# Specs index

| Doc | What it covers |
|-----|----------------|
| [001-architecture.md](001-architecture.md) | Process model, packages, request flow |
| [002-data-model.md](002-data-model.md) | SQLite schema, metrics, storage location |
| [003-providers.md](003-providers.md) | Provider interface, CLI invocation, event mapping |
| [004-ui.md](004-ui.md) | Layout, panels, HTTP API |
| [005-testing.md](005-testing.md) | The three test layers and how they are isolated |
| [006-sidebar-ordering.md](006-sidebar-ordering.md) | Sidebar ordering, archiving, and Tree placement |
| [007-session-closing.md](007-session-closing.md) | Closing, archiving, and restoring sessions |
| [008-project-workspaces.md](008-project-workspaces.md) | Per-project tab strips, tab groups, file diffs |
| [009-model-picker.md](009-model-picker.md) | Cross-provider model and favourite selection |
| [010-working-indicator.md](010-working-indicator.md) | Live turn progress in the transcript |
| [011-prompt-bar-usage.md](011-prompt-bar-usage.md) | Fuzzy models, context ring, subscription allowance |
| [012-session-queue.md](012-session-queue.md) | Queued prompts and project scheduling |
| [013-project-logs.md](013-project-logs.md) | The Logs pane, commit files, commit diffs |
| [014-transcript-messages.md](014-transcript-messages.md) | Copying messages, editing a prompt in place |

Status: sidebar ordering, session archiving, collapsible project session lists,
the project-scoped Tree, fuzzy go-to navigation, global conversation closing,
remembered open tabs, project-scoped closed-tab reopening, prompt and file
undo/redo, clickable usage details, and renaming a session in its row are implemented. Earlier Go, web-unit, and
targeted Sessions/Inspector browser runs pass; this prototype update does not
add or run tests.

Written but not yet run: stepping the tab strip with Ctrl+PageUp/PageDown, the
shortcut list under the sidebar, focusing the Sessions and Tree filters when
their pane is shown, a project page handing its tab over to the session started
from it, and the per-project tab strip — draggable tabs in three coloured
groups, Alt+A…H to switch project, the pulsing dot and shortcut letter on a
project row, and a per-session prompt draft. The accent and grey pickers in
Settings are the same.

Also written, not compiled: the file tab's three views. `Edit | Diff |
Preview` replaces `File | Diff` — an in-place editor with hand-written syntax
colouring, a `*` and a Save button over `PUT /api/projects/:id/file`, and a
Save / Don't save / Continue editing dialog on close; the diff as unified or
side by side; markdown and HTML preview. A file tab now carries its own status
bar instead of the conversation's prompt box.

None of it has been compiled: `make ui`, `go vet` and a browser check are
still to run.

Also changed, not compiled or run: workspace sessions now pass
`--permission-mode auto` instead of `acceptEdits`, so a Claude Code session can
run commands rather than only edit files. `go test ./app/internal/agent/...`
and a check that a session actually commits are still to run.

Claude Code sessions stream into the transcript, record per-turn token metrics,
and resume across turns. The Codex provider is implemented against the current
CLI JSONL contract and compiles; its picker exposes Astra, Sol, Terra, and Luna
with their supported reasoning efforts. Its billable live-turn check remains pending.
Several sessions can be open at once, each streaming into its own tab.
