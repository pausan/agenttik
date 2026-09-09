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
| [013-project-logs.md](013-project-logs.md) | The Commits pane, commit files, commit diffs |
| [014-transcript-messages.md](014-transcript-messages.md) | Copying messages, editing a prompt in place |
| [015-settings-shortcuts.md](015-settings-shortcuts.md) | Settings sections, the filter, editable chords |
| [016-file-tab-reuse.md](016-file-tab-reuse.md) | The temporary file tab a click reuses |
| [017-general-settings.md](017-general-settings.md) | The General section, and which key sends |
| [018-readme-icon.md](018-readme-icon.md) | Concise project overview and shared SVG icon |
| [019-file-watching.md](019-file-watching.md) | Tree and Changed following the disk as it moves |

Status: the tree and the changed list now follow the project folder as it
moves on disk — see 019 for the watcher and what it deliberately ignores.
The General settings section and the Enter / Ctrl+Enter swap, the
temporary file tab that a click reuses and a double click keeps, the Settings
rail with its fuzzy filter and editable shortcuts, the Commits pane and its commit
diffs, transcript message copying and prompt editing, queued prompts drawn in
the transcript with their waiting timer, and the Claude Code subscription
allowance are implemented and browser-checked — see 017, 016, 015, 013, 014,
012 and 011 for what each was verified against. Enqueue is Ctrl+Enter by
default, and General swaps it with Enter.
Sidebar ordering, session archiving, collapsible project session lists, the
project-scoped Tree, fuzzy go-to navigation, global conversation closing,
remembered open tabs, project-scoped closed-tab reopening, prompt and file
undo/redo, clickable usage details, renaming a session in its row, the
per-project tab strip, the file tab's three views, and queued prompts are all
implemented.

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (32) pass.
Two Go tests fail and did so before this work: `TestDoneReachesProjectTopic`
in runner and `TestReorderSessionsDrivesProjectOrder` in store. Two e2e cases
fail the same way, both driving the model picker as the `USelect` it stopped
being — see 015.

Claude Code sessions stream into the transcript, record per-turn token metrics,
report their context window and subscription allowance, and resume across
turns. The Codex provider is implemented against the current CLI JSONL
contract and its app-server allowance query answers live; its billable
live-turn check remains pending.
