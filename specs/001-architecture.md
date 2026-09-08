# Architecture

## Goal

Run agent coding sessions from one local web UI, against subscriptions you already
pay for, without going outside their terms.

The rule that shapes everything else: **agenttik never handles subscription
credentials.** It shells out to the official CLIs (`claude`, `codex`), which
authenticate with the accounts already configured on the machine
(`~/.claude`, `~/.codex/auth.json`). No token extraction, no replaying a
subscription session against the raw API. Key-based providers (OpenRouter,
direct APIs) come later as a separate provider kind that does hold keys.

Linux only for now.

## Shells

One binary, two ways to show the same UI:

- **Desktop (default)** — a Wails window. It starts the HTTP server on a random
  loopback port and points a WebKit view at it through a reverse proxy with
  `FlushInterval = -1`, so SSE still streams. Needs cgo, gtk3 and
  webkit2gtk; built with `-tags "desktop production webkit2_41"`.
- **Web (`--web`)** — the HTTP server only, prints its URL. Builds with
  `CGO_ENABLED=0` and no system dependencies.

A binary built without the `desktop` tag falls back to web mode with a notice
rather than failing, so `go build ./...` works anywhere.

## Process model

One Go binary. Fiber serves the HTTP API and the embedded static UI. Each agent
turn is a short-lived child process:

```
browser ──HTTP──> fiber ──> runner ──> exec: claude -p --output-format stream-json
   ^                          │                 (one process per turn)
   └──────SSE────── hub <─────┘
```

A turn is one prompt and everything the agent does in response. The child exits
when the turn ends; continuity comes from the provider's own session id
(`claude --resume <id>`, `codex exec resume <id>`), which we store.

Why one process per turn rather than one long-lived process per session:

- Restart-safe. Kill the server, sessions survive in SQLite and resume.
- Uniform. `codex exec` has no streaming stdin channel; `claude` does. Per-turn
  processes are the shape both providers support.
- Cheap. Idle sessions cost nothing. Dozens can be open at once.

The cost is per-turn CLI startup (~1s) and no mid-turn interaction. Approvals are
therefore decided up front by the session's permission mode, not asked mid-flight.

## Packages

| Package | Responsibility |
|---------|----------------|
| `cmd/agenttik` | flags, wiring, graceful shutdown |
| `internal/config` | data dir resolution (XDG), listen address |
| `internal/store` | SQLite access and migrations. No business logic |
| `internal/agent` | provider-neutral `Event`/`TurnRequest` types, `Provider` interface, registry |
| `internal/agent/claudecode` | Claude Code CLI adapter |
| `internal/agent/codex` | Codex CLI adapter (stub) |
| `internal/runner` | turn lifecycle: spawn, consume events, persist, fan out |
| `internal/server` | Fiber routes, SSE, project file access |
| `web` | embedded static UI |

Dependencies point one way: `server` -> `runner` -> `agent` + `store`. `agent`
knows nothing about HTTP or SQL.

## Concurrency

- One turn at a time per session; a second prompt while running is rejected.
- Any number of sessions run concurrently, including several in one project.
- The hub fans events out to SSE subscribers with buffered per-subscriber
  channels. A slow browser gets dropped, never blocks the turn.
