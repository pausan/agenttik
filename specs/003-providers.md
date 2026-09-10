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

`Run` starts the CLI and returns a channel that closes when the turn is over. The
channel carries provider-neutral events:

`session_started` (carries the provider session id), `text`, `thinking`,
`tool_use`, `tool_result`, `usage`, `done`, `error`.

Cancelling the context kills the process group, which is how "stop" works.

## Permission modes

Chosen per session; default `workspace`. A web UI cannot answer an interactive
approval prompt, so the posture is set before the turn starts.

| agenttik | claude | codex |
|----------|--------|-------|
| `plan` | `--permission-mode plan` | `--sandbox read-only` |
| `workspace` (default) | `--permission-mode auto` | `--sandbox workspace-write` |
| `full` | `--dangerously-skip-permissions` | `--dangerously-bypass-approvals-and-sandbox` |

`workspace` maps to claude's `auto`, the mode the IDE extensions use for their
Auto setting, and deliberately not to `acceptEdits`. Both auto-approve file
edits, but `acceptEdits` still asks before running a command; under `-p` there
is nobody to ask, so the call is refused. That made workspace sessions able to
edit files but never build, test, or commit — an agent told to commit its work
silently did not. `auto` approves ordinary work and still asks for the
destructive cases, so only those are refused.

If `auto` turns out to withhold something a session needs, the lever is
`--allowedTools` (a space- or comma-separated list such as `"Bash(git commit:*)
Edit"`) rather than widening the whole session to `full`.

## Claude Code

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
| `{"type":"result",...}` | `usage` + `done` |

Models are aliases (`fable`, `opus`, `sonnet`, `haiku`) so they track the latest
release without a code change. Efforts: `low`, `medium`, `high`, `xhigh`, `max`.

## Codex

Codex runs through the locally authenticated CLI, so agenttik never reads or
handles the account credentials. The invocation is:

```
codex exec --json --model <model> -c model_reasoning_effort=<effort> \
           --sandbox <mode> --cd <workdir>
codex exec resume <session-id> --json ...
```

The picker offers GPT-6 Astra, GPT-5.6 Sol, GPT-5.6 Terra, and GPT-5.6 Luna,
each with its documented context window and reasoning levels. Astra offers
`low` through `max`; Sol, Terra, and Luna also offer `none`. Model-specific
efforts are returned with the model record, so the UI never offers an effort
that the chosen model does not support.

The prompt is sent on stdin. `resume` uses the session's original sandbox
policy; the CLI does not accept a new `--sandbox` flag for that subcommand.
The process working directory remains the project's directory. The CLI's JSONL
stream maps as follows:

| Line | Mapped to |
|------|-----------|
| `{"type":"thread.started","thread_id":...}` | `session_started` |
| `{"type":"item.started","item":{"type":"command_execution",...}}` | `tool_use` |
| `{"type":"item.completed","item":{"type":"agent_message","text":...}}` | `text` |
| `{"type":"item.completed","item":{"type":"reasoning",...}}` | `thinking` |
| completed command, MCP, or web-search item | `tool_result` |
| `{"type":"turn.completed","usage":...}` | `usage` + `done` |
| `turn.failed` or `error` | `error` |

`input_tokens` includes the cached portion reported separately as
`cached_input_tokens`; the former is used as the current context size.

Status: implementation compiles against Codex CLI 0.147.0's command syntax and
the official JSONL event contract. A live subscription turn was not run because
it would consume account usage.

## Later: key-based providers

OpenRouter and direct APIs implement the same interface but talk HTTP instead of
spawning a process, and read keys from config. The `Provider` split above is
where that lands; nothing else changes.
