# Providers

## Interface

```go
type Provider interface {
    Name() string                  // "claude" | "codex"
    DisplayName() string
    Models() []Model
    Efforts() []string
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

## Codex — stub

Command shape is written but event parsing is not implemented; `Run` returns
`ErrNotImplemented`. The intended invocation:

```
codex exec --json --model <model> -c model_reasoning_effort=<effort> \
           --sandbox <mode> --cd <workdir>
codex exec resume <session-id> --json ...
```

Codex JSONL event names differ between CLI versions, so the parser needs to be
written against the installed version and pinned by a fixture test rather than
guessed.

## Later: key-based providers

OpenRouter and direct APIs implement the same interface but talk HTTP instead of
spawning a process, and read keys from config. The `Provider` split above is
where that lands; nothing else changes.
