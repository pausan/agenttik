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

Status: the Logs pane and its commit diffs, transcript message copying and
prompt editing, and the Claude Code subscription allowance are implemented and
browser-checked — see 013, 014 and 011 for what each was verified against.
Sidebar ordering, session archiving, collapsible project session lists, the
project-scoped Tree, fuzzy go-to navigation, global conversation closing,
remembered open tabs, project-scoped closed-tab reopening, prompt and file
undo/redo, clickable usage details, renaming a session in its row, the
per-project tab strip, the file tab's three views, and queued prompts are all
implemented.

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (32) pass.
Two Go tests fail and did so before this work: `TestDoneReachesProjectTopic`
in runner and `TestReorderSessionsDrivesProjectOrder` in store.

Claude Code sessions stream into the transcript, record per-turn token metrics,
report their context window and subscription allowance, and resume across
turns. The Codex provider is implemented against the current CLI JSONL
contract and its app-server allowance query answers live; its billable
live-turn check remains pending.
