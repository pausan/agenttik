# Providers

## CLI discovery

Providers and the Subscriptions panel find CLIs through the server's `PATH`.
Before starting providers, macOS and Linux desktop builds without a terminal
environment (`TERM` unset, empty or `dumb`) recover additional search directories
from the user's interactive login shell (`$SHELL`, defaulting to `/bin/zsh` on
macOS and `/bin/sh` on Linux). This includes shell-configured Homebrew and
version-manager paths when opened from Finder, the Dock or a desktop launcher.

The shell runs once in the user's home directory, with a five-second timeout.
Only `PATH` is imported; existing entries retain priority, new absolute entries
are deduplicated, and shell banners are ignored. Lookup failure is logged and
leaves the inherited path intact. Whether or not the shell answered, the usual
install directories that exist are appended last: `~/.local/bin` (Claude Code's
native installer), `~/.claude/local`, `/opt/homebrew/bin` and `/usr/local/bin`.
A slow or broken shell startup therefore cannot hide a natively installed
`claude`. CLI children inherit the recovered path too, so scripts can find
runtimes such as `node`.

Terminal environments, explicit `--web` launches, web-only builds and Windows
keep their inherited path. Remote clients and command-only invocations do not
start the shell. Tests simulate a desktop path, verify both discovery and CLI
execution, and cover failed, incomplete and timed-out shell probes, and the install
directories found without a shell.

## Interface

```go
type Provider interface {
    Name() string                  // subscription ID or "api-<service>"
    DisplayName() string
    Models() []Model               // each may define its own efforts
    Efforts() []string             // fallback for models without their own list
    Available() error              // CLI available or API connection enabled
    Run(ctx context.Context, req TurnRequest) (<-chan Event, error)
}
```

Additional interfaces are optional. `SmallModel` names the provider's lightest model,
which agenttik puts its own short questions to: naming a task from its first
prompt ([020](020-task-titles.md)) and saying what a finished one came to
([051](051-task-outcomes.md)). `Metered` answers one signed-in account's own
subscription allowance without ever holding its credentials
([011](011-prompt-bar-usage.md)); `MultiAccount` says where that account's
login is kept, so one machine can hold a company subscription and a personal
one for the same provider ([050](050-subscription-accounts.md)). A provider
that only volunteers an allowance mid-turn implements neither of the first two
and is read off its limits event instead.

`TurnRequest.AccountHome` is the login directory the turn runs on, empty for
the machine's own. Each provider applies it the way its CLI expects —
`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `copilot --config-dir` — and an empty one
inherits this process's environment untouched, so a machine with a single
subscription runs exactly the command it always ran.

`DirectAccount` exposes write-only key setup and execution preferences for
providers supporting direct subscription use. Currently this is OpenCode Go.

API-key providers use separate `api-*` IDs and profile-scoped connection files
([078](078-api-providers.md)). They run through the shared local tool runtime
([077](077-direct-api-runtime.md)); subscription credentials are not reused.

`Run` starts the CLI or direct HTTP tool loop and returns a channel that closes when the turn is over. The
channel carries provider-neutral events:

`session_started` (carries the provider session id), `text`, `thinking`,
`tool_use`, `tool_result`, `usage`, `done`, `error`, and `approval` /
`approval_resolved` ([082](082-tool-approvals.md)).

Cancelling the context kills the process group, which is how "stop" works.

## Permission modes

Chosen per task; default `workspace`. The posture is set before the turn
starts. What it leaves open, Claude Code and Codex ask the user about mid-turn
([082](082-tool-approvals.md)); the other providers refuse it.

| agenttik | claude | Codex app-server sandbox | Codex approval policy |
|----------|--------|--------------------------|-----------------------|
| `plan` | `--permission-mode plan` | `read-only` | `never` |
| `workspace` (default) | `--permission-mode auto` | `danger-full-access` | `on-request`, reviewer `user` |
| `full` | `--dangerously-skip-permissions` | `danger-full-access` | `never` |

Codex runs with its sandbox disabled and full permissions by default for now,
including resumed tasks. On Linux hosts that restrict unprivileged user
namespaces (Ubuntu's AppArmor default), Codex's bubblewrap sandbox cannot start
and `workspace-write` asks before every command, even inside the project, so
the sandbox stays off. Workspace tasks still ask, so what Codex escalates on
its own — MCP tool calls, rule matches — reaches the user; the reviewer is set
to `user` because a personal config may route approvals to a reviewing model
(`approvals_reviewer = "auto_review"`). The exec path uses
`--dangerously-bypass-approvals-and-sandbox` for both modes. Explicit `plan`
mode remains read-only.

`workspace` maps to claude's `auto`, the mode the IDE extensions use for their
Auto setting, and deliberately not to `acceptEdits`. Both auto-approve file
edits, but `acceptEdits` still asks before running every command, which would
put each build and test to the user. `auto` approves ordinary work and asks
only for the destructive cases.

If `auto` turns out to withhold something a task needs, the lever is
`--allowedTools` (a space- or comma-separated list such as `"Bash(git commit:*)
Edit"`) rather than widening the whole task to `full`.

## Claude Code

The account is whichever `CLAUDE_CONFIG_DIR` names, and everything about one
login — the credential, the settings, the history — is under it.

```
claude -p --output-format stream-json --include-partial-messages --verbose \
       --input-format stream-json --permission-prompt-tool stdio \
       --model <model> --effort <effort> --permission-mode <mode> \
       --session-id <uuid> | --resume <uuid>
```

The prompt goes in on stdin as a stream-json `user` line, never as an argv
element; stdin stays open for approval answers ([082](082-tool-approvals.md)). `--session-id` is used on
a task's very first turn (we choose the uuid, so both sides name the
conversation the same), `--resume` on every turn after. A turn that has no
thread to resume although the task has already run — a provider, subscription
or tool-access change resets the thread — passes neither: the CLI already keeps
a conversation under the task's id and refuses to be given it again ("Session
ID ... is already in use"), so it picks the new one itself and reports it in
`init`.

Task turns run with `MCP_CONNECTION_NONBLOCKING=0`. Under `-p` the CLI otherwise
connects MCP servers in the background, so a claude.ai connector still
connecting when the first request goes out looks disconnected to the model for
the whole turn. The wait is capped by the CLI's `MCP_CONNECT_TIMEOUT_MS` (5s by
default). Metadata requests skip it, and a value set in agenttik's own
environment wins.

Events consumed from stdout JSONL:

| Line | Mapped to |
|------|-----------|
| `{"type":"system","subtype":"init","session_id":...}` | `session_started` |
| `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta"}}}` | `text` |
| `...{"delta":{"type":"thinking_delta"}}` | `thinking` |
| `{"type":"assistant","message":{"content":[{"type":"tool_use",...}]}}` | `tool_use` |
| `{"type":"user","message":{"content":[{"type":"tool_result",...}]}}` | `tool_result` |
| `{"type":"control_request","request":{"subtype":"can_use_tool",...}}` | `approval` |
| `{"type":"control_cancel_request",...}` | `approval_resolved` |
| `{"type":"system","subtype":"background_tasks_changed","tasks":[...]}` | nothing; counts running background tasks |
| `{"type":"result",...}` | `done` with running turn totals; stdin is closed unless background tasks run |

A result is one reply, not always the end of the turn. When the agent ends a
reply with background tasks running (`run_in_background` commands, background
agents) and says it will wait, stdin stays open. When a task finishes, the CLI
wakes the agent itself (`task_notification`, a fresh `init`) and it replies
again, possibly starting more background work. Stdin closes at the first
result with no background task running, and the turn ends when the CLI exits.
Until then the task shows as working; Stop kills the CLI and its tasks.
Closing stdin earlier makes the CLI kill the tasks and never wake the agent.
Each result's `usage` covers its own reply and is summed; `total_cost_usd` is
already the process total, so the last one is kept. The runner stores each
reply as its own message.

Models are aliases (`fable`, `opus`, `sonnet`, `haiku`) so they track the latest
release without a code change. Labels include the version reported by the
installed CLI's initialization metadata, such as `Opus 5.5`. A prompt-free
initialization query runs outside the project with safe mode and user settings
only. It runs at startup and in the background, never inside a request: a
request reads the cached labels, and one finding them over ten minutes old
starts a refresh and answers with what it has. A failed refresh retains the
last successful labels. Until a version is known, the label says `version unknown`.
Model IDs remain aliases, preserving saved selections and favorites. This
catalogue describes the machine's default Claude configuration; account-specific
overrides and runtime fallback can still change the model used for a turn.
Efforts: `low`, `medium`, `high`, `xhigh`, `max`.

An assistant line without `parent_tool_use_id` belongs to the root agent: its
message usage updates the main context and the main-agent partition. A line
with that id belongs to a child agent, so its usage goes into the subagent
partition and its independent context never moves the root gauge. The result
line remains authoritative for whole-task totals; any difference between that
total and the message partitions is unattributed.

## Codex

Codex runs through the locally authenticated CLI, so agenttik never reads or
handles the account credentials. Normal task turns use its stdio app-server:

```
codex -c features.default_mode_request_user_input=true app-server --stdio
initialize
thread/start | thread/resume
turn/start
```

The picker offers GPT-6 Astra, GPT-5.6 Sol, GPT-5.6 Terra, and GPT-5.6 Luna,
each with its documented context window and reasoning levels. Astra offers
`low` through `max`; Sol, Terra, and Luna also offer `none`. Model-specific
efforts are returned with the model record, so the UI never offers an effort
that the chosen model does not support.

The prompt is a text input on `turn/start`. Start and resume both receive the
selected model, project directory, sandbox mode, and the approval policy above;
`thread/resume` excludes historical turns from its response. The feature flag
gives the model its question tool outside Codex's plan mode; an unknown flag
is ignored with a warning. Requests app-server makes of the client are
described in [082](082-tool-approvals.md). The
thread id returned by start or resume is the provider session id.

The app-server's JSONL stream maps as follows:

| Notification | Mapped to |
|--------------|-----------|
| thread response / `thread/started` | provider session and child thread identity |
| `item/agentMessage/delta` | root `text` |
| `item/reasoning/summaryTextDelta` | root `thinking` |
| root `item/started` tool items | `tool_use` |
| root `item/completed` tool items | `tool_result` |
| `thread/tokenUsage/updated` | root context plus root/child task usage |
| root `turn/completed` | `done`, and `error` when failed |

Token usage is tracked by thread. `last.inputTokens` from the root thread is
the current main context; task totals are the current-turn deltas of root and
child `total` counters. Cached input remains a separately reported subset.
When a persisted root or child is resumed, its existing total is removed from
the delta. A distinct child thread seen through collaboration items,
`thread/started`, or usage counts once as a subagent.

Agenttik's own title and outcome requests remain isolated
`codex exec --json --ephemeral` calls. Its `turn.completed.usage` is an
aggregate, not a last-prompt measurement, and therefore never feeds the
context gauge.

Implemented against Codex CLI 0.153.4's generated app-server schema and the
official JSONL contracts. A live subscription turn has never been run, because
it would spend real account usage; request construction, usage tracking, event
mapping and the app-server allowance query are covered without one.

## GitHub Copilot

The third provider, through the locally authenticated `copilot` CLI. Its
session contract, permission mapping and quota query are
[031](031-copilot-subscription.md); its model list is asked of the CLI rather
than written down here, because both the catalogue and the account's
entitlement change without a release — see [036](036-copilot-model-list.md).

## Fake, for the browser tests

A test-only provider, `app/internal/agent/fake`, implements the same interface
against no process and no account: a turn is scripted from directive lines in
the prompt instead. It exists only for the end-to-end suite and is registered
only when `AGENTTIK_FAKE_PROVIDER` is set, which nothing but
`e2e/fixtures.js` does — see [005](005-testing.md).

## OpenCode Go

The `opencode` provider offers the Go subscription with an optional CLI and a
direct coding-agent path. New conversations prefer an installed CLI; users
can force Direct in Settings. The account key, model set, event mapping,
permissions, session persistence and provider policy review are documented in
[065](065-opencode-subscription.md).
