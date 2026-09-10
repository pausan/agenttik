# Architecture

## Goal

Run coding agents from one local web UI, against subscriptions you already pay
for, without going outside their terms.

The rule that shapes everything else: **agenttik never handles subscription
credentials.** It shells out to the official CLIs (`claude`, `codex`), which
authenticate with the accounts already configured on the machine
(`~/.claude`, `~/.codex/auth.json`). No token extraction, no replaying a
subscription session against the raw API. Key-based providers (OpenRouter,
direct APIs) come later as a separate provider kind that does hold keys.

The desktop shell supports Linux, macOS amd64, and Windows amd64.

## Shells

One binary, two ways to show the same UI:

- **Desktop (default)** — a Wails window. It starts the HTTP server on a random
  loopback port and points a WebKit view at it through a reverse proxy with
  `FlushInterval = -1`, so SSE still streams. Linux needs cgo, gtk3 and
  webkit2gtk and builds with `-tags "desktop production webkit2_41"`; macOS
  and Windows use Wails' native platform backends with
  `-tags "desktop production"`. Settings ›
  Server can additionally expose that same server on a chosen host and port,
  through a second, independent proxy the window itself never touches — see
  [043](043-exposed-server.md).
- **Web (`--web`)** — the HTTP server only, prints its URL. Builds with
  `CGO_ENABLED=0` and no system dependencies beyond node for the UI.

Either shell holds an exclusive lock on its data directory, so a second launch
raises the window already open and exits instead of running a rival copy over
the same database — see [039](039-single-instance.md).

A binary built without the `desktop` tag falls back to web mode with a notice
rather than failing, so `go build ./...` works anywhere. The UI is a Vite build
(`make ui`), and a binary made without it says so rather than serving a blank
page — see [004](004-ui.md).

## Process model

One Go binary. Fiber serves the HTTP API and the embedded UI. Each agent turn
is a short-lived child process:

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

Three top-level directories hold code: `app/` is the Go application, `web/` the
Vue UI and its embed glue, `e2e/` the browser tests ([005](005-testing.md)).

| Package | Responsibility |
|---------|----------------|
| `app/cmd/agenttik` | flags, wiring, graceful shutdown |
| `app/internal/config` | data dir resolution (XDG), listen address |
| `app/internal/single` | the one-instance-per-data-dir lock and the address behind it |
| `app/internal/store` | SQLite access and migrations. No business logic |
| `app/internal/agent` | provider-neutral `Event`/`TurnRequest` types, `Provider` interface, registry |
| `app/internal/agent/claudecode` | Claude Code CLI adapter |
| `app/internal/agent/codex` | Codex CLI adapter |
| `app/internal/agent/copilot` | GitHub Copilot CLI adapter ([031](031-copilot-subscription.md)) |
| `app/internal/process` | killing a child's whole process group, which is how stop works |
| `app/internal/runner` | turn lifecycle: spawn, consume events, persist, fan out; the schedule and retry clocks |
| `app/internal/server` | Fiber routes, SSE, project file access |
| `app/internal/netserver` | the second listener Settings › Server exposes ([043](043-exposed-server.md)) |
| `web` | the Vue source, and `embed.go` compiling `dist/` into the binary |

Dependencies point one way: `server` -> `runner` -> `agent` + `store`. `agent`
knows nothing about HTTP or SQL.

## Shutdown

Closing the window, or Ctrl-C in web mode, has to end the process. A live SSE
stream never finishes by itself and Fiber waits for every open connection, so
`Server.Shutdown` closes a `closing` channel first — that releases the stream
handlers — and only then waits, with a 3s cap, for what is left. Without the
first step the window disappears while the process stays alive holding the port
and the database, which is what the desktop shell's close button used to do.

## Concurrency

- One turn at a time per task; a second prompt while one runs is queued rather
  than run beside it ([012](012-task-queue.md)).
- Any number of tasks run concurrently, including several in one project.
- The hub fans events out to SSE subscribers with buffered per-subscriber
  channels. A slow browser gets dropped, never blocks the turn.
- One subscriber channel may be registered under many topics, which is how a UI
  with a dozen tabs open watches them all over one connection. A browser holds
  only a handful of connections to one origin, so a stream per tab would stall
  the API — see [004](004-ui.md#live-updates).
