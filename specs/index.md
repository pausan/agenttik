# Specs index

Each file describes the design as it stands. When behaviour changes the file is
rewritten, not appended to, so there is no history here — `git log` is the
history. Vocabulary follows [038](038-task-naming.md): **task** for the thing on
screen, **session** for the row, the route or the CLI flag.

## The whole app

| Doc | What it covers |
|-----|----------------|
| [001](001-architecture.md) | Process model, packages, request flow |
| [002](002-data-model.md) | SQLite schema, metrics, storage location |
| [003](003-providers.md) | Provider interface, CLI invocation, event mapping |
| [004](004-ui.md) | Layout, panels, shortcuts, HTTP API |
| [005](005-testing.md) | The three test layers and how they are isolated |

## Projects and tasks

| Doc | What it covers |
|-----|----------------|
| [006](006-sidebar-ordering.md) | Sidebar ordering, archiving, and Tree placement |
| [007](007-task-closing.md) | Closing, archiving, and restoring a task |
| [008](008-project-workspaces.md) | Per-project tab strips, the context tab, file diffs |
| [021](021-stop-active-tasks.md) | Stopping an active or scheduled task from the sidebar |
| [025](025-blank-task-reuse.md) | Starting a task lands on an untitled one wherever it is |
| [030](030-project-tasks.md) | Every task of a project in one paged list, behind a focused filter |
| [038](038-task-naming.md) | What a task is called, on screen and on the wire |
| [040](040-editable-project-path.md) | Repointing a project at a moved folder, by typing or by browsing |
| [041](041-project-archiving.md) | Putting a project away without deleting it, and the Settings list it waits in |
| [042](042-sidebar-project-rows.md) | A project added lands last, the rule between projects, and the letter that folds one |

## Prompting and turns

| Doc | What it covers |
|-----|----------------|
| [009](009-model-picker.md) | Cross-provider model and favourite selection |
| [010](010-working-indicator.md) | Live turn progress in the transcript |
| [011](011-prompt-bar-usage.md) | Fuzzy models, context ring, every subscription window |
| [012](012-task-queue.md) | Queued prompts and project scheduling |
| [020](020-task-titles.md) | Instant first-line title, refined by an isolated small model |
| [024](024-instant-prompt-echo.md) | Drawing sent and queued prompts on the keypress |
| [028](028-scheduled-jobs.md) | Repeating a prompt on a clock, and the tasks it spawns |
| [033](033-prompt-model-picker.md) | Bounded, collapsible provider groups in the prompt model picker |
| [045](045-provider-outage-retry.md) | A prompt whose provider was away waits, and starts itself when it answers |

## Reading the transcript and the files

| Doc | What it covers |
|-----|----------------|
| [013](013-project-logs.md) | The Commits pane, commit files, commit diffs |
| [014](014-transcript-messages.md) | Copying messages, editing a prompt in place |
| [016](016-file-tab-reuse.md) | The temporary file tab a click reuses |
| [019](019-file-watching.md) | Tree and Changed following the disk as it moves |
| [022](022-transcript-file-links.md) | Clicking a path in a reply to open the file at its line |
| [023](023-tool-call-groups.md) | Collapsing a run of tool calls to its latest |
| [026](026-image-preview.md) | Previewing an SVG or an image, and diffing two pictures |
| [027](027-ignored-files-in-the-tree.md) | Every file in the Tree, the ignored ones grey |

## Settings, shell and packaging

| Doc | What it covers |
|-----|----------------|
| [015](015-settings-shortcuts.md) | Settings sections, the filter, editable chords |
| [017](017-general-settings.md) | The General section, and which key sends |
| [018](018-readme-icon.md) | The project overview, and the one SVG the README, favicon and window share |
| [029](029-desktop-proxy-logging.md) | Why the window logged broken pipes, and the abort it recovers instead |
| [032](032-single-tab-close-shortcut.md) | Ctrl+W closes one active tab, including when held |
| [037](037-cross-platform-releases.md) | Push builds and tagged Linux, macOS, and Windows releases |
| [039](039-single-instance.md) | One app per data directory, and the window a second launch raises |
| [043](043-exposed-server.md) | Serving the window's UI to a browser too, on a chosen host and port |
| [044](044-command-line.md) | Long options, `--help`, and the version a release tag builds in |

## Providers

| Doc | What it covers |
|-----|----------------|
| [031](031-copilot-subscription.md) | GitHub Copilot CLI tasks and account quota |
| [034](034-claude-update-retry.md) | Retrying Claude Code startup during a self-update |
| [035](035-missing-work-dir.md) | Reporting a renamed or deleted project folder as itself |
| [036](036-copilot-model-list.md) | Asking the Copilot CLI which models the account has |

## Status

Claude Code and GitHub Copilot are exercised end to end: they stream into the
transcript, record per-turn token metrics, report their context window and
subscription allowance, and resume across turns. Codex is implemented against
the current CLI JSONL contract and its app-server allowance query answers live,
but no live subscription turn has ever been run through it.

`go build ./...`, `go vet ./...`, `make ui`, the 33 web unit tests and the full
Playwright suite all pass. Two Go tests fail: `TestDoneReachesProjectTopic` in
`runner` and `TestReorderSessionsDrivesProjectOrder` in `store`. See
[005](005-testing.md).

[review-notes.md](review-notes.md) lists what these specs and the code still
disagree about, for a human to settle.
