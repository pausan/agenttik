# agenttik

A local control panel for agent coding sessions. Run many sessions across many
projects, from one window, on the subscriptions you already pay for.

Go + Fiber backend, SQLite, a Vue 3 + Nuxt UI frontend, Wails desktop shell.
Linux only for now.

## Why

`claude` and `codex` are excellent in a terminal and awkward when you want five
of them at once. agenttik gives them a shared home: projects you register once,
sessions you can leave and come back to, and honest per-turn numbers on what
each one cost you.

## Within terms

agenttik never handles subscription credentials. It shells out to the official
CLIs, which authenticate with the accounts already configured on your machine
(`~/.claude`, `~/.codex/auth.json`). Nothing is extracted, proxied or replayed
against the raw API. If a CLI can do it, agenttik can drive it; if it cannot,
agenttik does not work around it.

Key-based providers (OpenRouter, direct APIs) are planned as a separate provider
kind, where holding a key is the point.

## Status

| | |
|---|---|
| Claude Code provider | works — streaming, resume, token metrics |
| Codex provider | stub — command shape written, event parsing not implemented |
| OpenRouter / direct APIs | not started |

## Requirements

- Go 1.22+
- Node 20+ (the UI is a Vite build)
- [`claude`](https://claude.com/claude-code) on `PATH`, already logged in
- For the desktop build: `gcc`, `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`
  (`make deps` installs these on Debian/Ubuntu)

## Build and run

```sh
make build      # UI + desktop app  -> bin/agenttik
make run        # build and open the window

make build-web  # UI + server only  -> bin/agenttik-web  (CGO_ENABLED=0)
make run-web    # build and serve on http://127.0.0.1:7717

make ui         # just the UI    -> web/dist
make test
```

Both build targets compile the UI first, so a plain `make build` is all you
need. To work on the frontend, run `make run-web` in one shell and `make ui-dev`
in another: Vite serves the UI on :5173 with hot reload and proxies the API to
the Go server.

Flags:

```
--web              serve the web UI only, no desktop window
--addr HOST:PORT   listen address for web mode (default 127.0.0.1:7717)
--data-dir DIR     where agenttik.db lives (default $XDG_DATA_HOME/agenttik)
```

The server binds to loopback and has no authentication. It is a local tool; do
not expose it.

## Using it

1. **Add a project** — a name and a folder. The folder is the agent's working
   directory. Nothing is copied.
2. **Start a session** — pick provider, model, effort and permission mode.
3. **Prompt** — the reply streams into the transcript. Star a model + effort
   combination and it sorts to the top of the picker next time.

The right panel carries **Stats** (tokens, cost, agent time, turn count),
**Changed** (files git reports as edited) and **Tree** (the repo's files).
Clicking a file opens it as a tab beside the conversation.

## Permission modes

An agent driven from a web UI cannot answer an interactive approval prompt, so
the posture is chosen per session, before the turn starts.

| Mode | Claude Code | Codex |
|------|-------------|-------|
| Read only | `--permission-mode plan` | `--sandbox read-only` |
| Edit inside the project (default) | `--permission-mode acceptEdits` | `--sandbox workspace-write` |
| No guardrails | `--dangerously-skip-permissions` | `--dangerously-bypass-approvals-and-sandbox` |

"No guardrails" lets the agent run anything as your user. It is there because it
is sometimes what you want; it is not the default for a reason.

## How it works

Each turn is one short-lived CLI process:

```
browser ──HTTP──> fiber ──> runner ──> exec: claude -p --output-format stream-json
   ^                          │                 (one process per turn)
   └──────SSE────── hub <─────┘
```

Continuity comes from the provider's own session id, stored and passed back as
`--resume`. That keeps the server restart-safe, keeps idle sessions free, and
works the same for both CLIs. The cost is roughly a second of CLI startup per
turn and no mid-turn interaction.

State lives in SQLite at `$XDG_DATA_HOME/agenttik/agenttik.db`. Token counts come
from the providers' own accounting, never an estimate.

## Layout

```
app/                the Go application
  cmd/agenttik      flags, wiring, desktop vs web shell
  internal/config   data dir and listen address
  internal/store    SQLite schema, migrations, queries
  internal/agent    provider contract; claudecode/ and codex/ adapters
  internal/runner   turn lifecycle and SSE fan-out
  internal/server   Fiber routes, SSE, project file access
web/                the Vue UI (Vite); dist/ is built and embedded by embed.go
specs/              design notes — start at specs/index.md
```

## License

Not yet chosen.
