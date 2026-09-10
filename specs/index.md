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
| [018-readme-icon.md](018-readme-icon.md) | Concise project overview, and the one SVG the README, favicon and window share |
| [019-file-watching.md](019-file-watching.md) | Tree and Changed following the disk as it moves |
| [020-queued-session-titles.md](020-queued-session-titles.md) | Isolated small-model titles for queued sessions |
| [021-stop-active-sessions.md](021-stop-active-sessions.md) | Stopping active or scheduled sessions from the sidebar |
| [022-transcript-file-links.md](022-transcript-file-links.md) | Clicking a path in a reply to open the file at its line |
| [023-tool-call-groups.md](023-tool-call-groups.md) | Collapsing a run of tool calls to its latest |
| [024-instant-prompt-echo.md](024-instant-prompt-echo.md) | Drawing a sent prompt on the keypress, not on the reply |
| [025-blank-session-reuse.md](025-blank-session-reuse.md) | Starting a session lands on an untitled one wherever it is |
| [026-image-preview.md](026-image-preview.md) | Previewing an SVG or an image, and diffing two pictures |
| [027-ignored-files-in-the-tree.md](027-ignored-files-in-the-tree.md) | Every file in the Tree, the ignored ones grey |

Status: the Tree now lists every file in the project, with what .gitignore
covers drawn grey, sorted last and its folders shut — see 027. The Sessions tab
now reaches back one hour by default and keeps the
window it was left on across launches — see 004. A file tab now renders pictures. An SVG previews and keeps its editor
and its text diff; a PNG, JPEG, GIF, WEBP, AVIF, BMP, ICO or APNG previews,
has no editor at all, and diffs as the two images side by side — see 026.
Starting a session now lands on an untitled conversation of the
project wherever it is, in a tab or not, instead of only when that one is in
front — see 025. Pressing Enter now draws the prompt at once, ahead of the request
that starts the turn — see 024. A run of consecutive tool calls now collapses to its latest, with
a button that opens the run and closes it again — see 023. A path an agent
names in its reply now opens the file in a tab, at
the line it named — see 022. The tree and the changed list follow the project
folder as it moves on disk — see 019 for the watcher and what it deliberately
ignores.
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

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (33) pass.
Two Go tests fail and did so before this work: `TestDoneReachesProjectTopic`
in runner and `TestReorderSessionsDrivesProjectOrder` in store. Two e2e cases
fail the same way, both driving the model picker as the `USelect` it stopped
being — see 015.

Claude Code sessions stream into the transcript, record per-turn token metrics,
report their context window and subscription allowance, and resume across
turns. The Codex provider is implemented against the current CLI JSONL
contract and its app-server allowance query answers live; its billable
live-turn check remains pending.
