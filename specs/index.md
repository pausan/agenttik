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
| [011-prompt-bar-usage.md](011-prompt-bar-usage.md) | Fuzzy models, context ring, every subscription window |
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
| [024-instant-prompt-echo.md](024-instant-prompt-echo.md) | Drawing sent and queued prompts on the keypress |
| [025-blank-session-reuse.md](025-blank-session-reuse.md) | Starting a session lands on an untitled one wherever it is |
| [026-image-preview.md](026-image-preview.md) | Previewing an SVG or an image, and diffing two pictures |
| [027-ignored-files-in-the-tree.md](027-ignored-files-in-the-tree.md) | Every file in the Tree, the ignored ones grey |
| [028-scheduled-jobs.md](028-scheduled-jobs.md) | Repeating a prompt on a clock, and the sessions it spawns |
| [029-desktop-proxy-logging.md](029-desktop-proxy-logging.md) | Why the window logged broken pipes, and the abort it recovers instead |
| [030-project-tasks.md](030-project-tasks.md) | Every task of a project in one paged list, behind a focused filter |
| [031-copilot-subscription.md](031-copilot-subscription.md) | GitHub Copilot CLI sessions and account quota |
| [032-single-tab-close-shortcut.md](032-single-tab-close-shortcut.md) | Ctrl+W closes one active tab, including when held |
| [033-prompt-model-picker.md](033-prompt-model-picker.md) | Bounded, collapsible provider groups in the prompt model picker |
| [034-claude-update-retry.md](034-claude-update-retry.md) | Retrying Claude Code startup during a self-update |
| [035-missing-work-dir.md](035-missing-work-dir.md) | Reporting a renamed or deleted project folder as itself |
| [036-copilot-model-list.md](036-copilot-model-list.md) | Asking the Copilot CLI which models the account has |

Status: A project page now holds every task it has in one list under Tasks —
open ones black, archived ones grey — with the cursor in the filter that
narrows both, drawn 25 rows to a page with 10, 50 and 100 on offer; see 030. An unsent prompt now outlives the tab it was typed in,
so changing focus or starting another task leaves the text where it was; see
004. The prompt bar reads the subscription allowance again whenever the task
or the model changes, and no longer repeats "running…" next to the header
badge that already says so. Its allowance panel now draws each window as a
name, a figure, a bar and a reset rather than one wrapping line; see 011. GitHub Copilot's model list now comes from the CLI instead of a
hand-written list that had gone stale in both directions — see 036. The prompt
model picker now has a bounded scrolling viewport, collapsible
provider groups and provider context in filtered results — see 033. Claude Code startup retries once if its updater briefly removes the executable — see 034. A turn whose project folder is gone now says so instead of blaming the CLI — see 035. Queued prompts now retain, show and let users change their individual
provider, model and effort before starting — see 012. The desktop window no
longer logs a broken pipe every time the UI reopens its event stream — see
029. A commit row in the Commits pane now shows only hash, author and time,
and right-clicking one copies its hash — see 013. A prompt can be scheduled to repeat — every X hours Y minutes, or
daily, weekly or monthly at a time — for a number of runs or forever; each run
is a fresh session that leaves the project list when its turn ends, a run that
comes due behind its own predecessor is skipped rather than queued, and the
schedule pauses before it can be archived. Implemented and browser-checked —
see 028. The Tree now lists every file in the project, with what .gitignore
covers drawn grey, sorted last and its folders shut — see 027. The Sessions tab
now reaches back one hour by default and keeps the
window it was left on across launches — see 004. A file tab now renders pictures. An SVG previews and keeps its editor
and its text diff; a PNG, JPEG, GIF, WEBP, AVIF, BMP, ICO or APNG previews,
has no editor at all, and diffs as the two images side by side — see 026.
Starting a session now lands on an untitled conversation of the
project wherever it is, in a tab or not, instead of only when that one is in
front — see 025. Pressing Enter or Ctrl+Enter now draws the prompt at once,
ahead of title work and the server response — see 024. A run of consecutive tool calls now collapses to its latest, with
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
Two Go tests fail: `TestDoneReachesProjectTopic` in runner and
`TestReorderSessionsDrivesProjectOrder` in store. The e2e suite has not been
carried through the sessions-to-tasks rename — it still asks for a
`New session` button, an `Untitled session` row and a `Sessions` stats label —
so most of it fails at `HEAD` and needs rewriting against the current
labels.

Claude Code sessions stream into the transcript, record per-turn token metrics,
report their context window and subscription allowance, and resume across
turns. The Codex provider is implemented against the current CLI JSONL
contract and its app-server allowance query answers live; its billable
live-turn check remains pending. GitHub Copilot sessions use the official CLI
JSONL contract and its read-only headless quota query; see 031.
| [037-linux-releases.md](037-linux-releases.md) | Tagged Linux amd64 and arm64 desktop builds and releases |
| [038-task-naming.md](038-task-naming.md) | Task naming and hover-popup content |
