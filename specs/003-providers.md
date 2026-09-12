# Providers

## Interface

```go
type Provider interface {
    Name() string                  // "claude" | "codex" | "copilot"
    DisplayName() string
    Models() []Model               // each may define its own efforts
    Efforts() []string             // fallback for models without their own list
    Available() error              // binary on PATH and usable
    Run(ctx context.Context, req TurnRequest) (<-chan Event, error)
}
```

Three halves are optional. `SmallModel` names the provider's lightest model,
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

`Run` starts the CLI and returns a channel that closes when the turn is over. The
channel carries provider-neutral events:

`session_started` (carries the provider session id), `text`, `thinking`,
`tool_use`, `tool_result`, `usage`, `done`, `error`.

Cancelling the context kills the process group, which is how "stop" works.

## Permission modes

Chosen per task; default `workspace`. A web UI cannot answer an interactive
approval prompt, so the posture is set before the turn starts.

| agenttik | claude | Codex app-server sandbox |
|----------|--------|--------------------------|
| `plan` | `--permission-mode plan` | `read-only` |
| `workspace` (default) | `--permission-mode auto` | `workspace-write` |
| `full` | `--dangerously-skip-permissions` | `danger-full-access` |

`workspace` maps to claude's `auto`, the mode the IDE extensions use for their
Auto setting, and deliberately not to `acceptEdits`. Both auto-approve file
edits, but `acceptEdits` still asks before running a command; under `-p` there
is nobody to ask, so the call is refused. That made workspace tasks able to
edit files but never build, test, or commit — an agent told to commit its work
silently did not. `auto` approves ordinary work and still asks for the
destructive cases, so only those are refused.

If `auto` turns out to withhold something a task needs, the lever is
`--allowedTools` (a space- or comma-separated list such as `"Bash(git commit:*)
Edit"`) rather than widening the whole task to `full`.

## Claude Code

The account is whichever `CLAUDE_CONFIG_DIR` names, and everything about one
login — the credential, the settings, the history — is under it.

```
claude -p --output-format stream-json --include-partial-messages --verbose \
       --model <model> --effort <effort> --permission-mode <mode> \
       --session-id <uuid> | --resume <uuid>
```

The prompt goes in on stdin, never as an argv element. `--session-id` is used on
the first turn (we choose the uuid), `--resume` on every turn after.

Events consumed from stdout JSONL:

| Line | Mapped to |
|------|-----------|
| `{"type":"system","subtype":"init","session_id":...}` | `session_started` |
| `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta"}}}` | `text` |
| `...{"delta":{"type":"thinking_delta"}}` | `thinking` |
| `{"type":"assistant","message":{"content":[{"type":"tool_use",...}]}}` | `tool_use` |
| `{"type":"user","message":{"content":[{"type":"tool_result",...}]}}` | `tool_result` |
| `{"type":"result",...}` | `done` with task totals |

Models are aliases (`fable`, `opus`, `sonnet`, `haiku`) so they track the latest
release without a code change. Efforts: `low`, `medium`, `high`, `xhigh`, `max`.

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
codex app-server --stdio
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
selected model, project directory, sandbox mode, and a noninteractive approval
policy; `thread/resume` excludes historical turns from its response. The
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

A fourth provider, `app/internal/agent/fake`, implements the same interface
against no process and no account: a turn is scripted from directive lines in
the prompt instead. It exists only for the end-to-end suite and is registered
only when `AGENTTIK_FAKE_PROVIDER` is set, which nothing but
`e2e/fixtures.js` does — see [005](005-testing.md).

## Later: key-based providers

OpenRouter and direct APIs implement the same interface but talk HTTP instead of
spawning a process, and read keys from config. The `Provider` split above is
where that lands; nothing else changes.
