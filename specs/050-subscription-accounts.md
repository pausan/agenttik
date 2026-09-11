# Subscriptions

One machine is often signed in to two accounts for the same provider: the
company Claude subscription and a personal one, a work ChatGPT and a private
one. A **subscription** here is a name and the directory its CLI keeps that
login in. Every task, queued prompt and job names the subscription it runs on,
every model control says which one it would spend, and Settings is where they
are added, chosen and signed in.

The credential boundary does not move. agenttik hands a CLI a path and the CLI
reads and signs with whatever is inside it, exactly as it does for the account
it was already using — see [003](003-providers.md).

## The system account

Account **0** is not a row in any table. It is the CLI as the machine already
has it: `~/.claude`, `~/.codex`, `~/.copilot`, or wherever the environment
already pointed them. It is what every task ran on before this existed and
what a task runs on when nothing else is chosen, so an existing database needs
no backfill, a fresh one needs no seed, and a single-subscription machine runs
exactly the commands it ran before — `agent.HomeEnv` returns nil for it, which
is `exec`'s own "inherit my environment unchanged".

It is called **System** on screen, since it sits beside the named ones in every
list. It cannot be renamed, moved or removed; it can be signed in, and it can
stop being the default.

## How a directory selects an account

| Provider | What carries it | Where the login is |
|----------|-----------------|--------------------|
| Claude Code | `CLAUDE_CONFIG_DIR` | `<dir>/.credentials.json` |
| Codex | `CODEX_HOME` | `<dir>/auth.json` |
| GitHub Copilot | `--config-dir` | the machine's vault, keyed per account; `<dir>/config.json` records which |

Copilot is the odd one, and the weakest of the three: its token goes into the
OS vault (keychain, keyring, credential manager) under the account it belongs
to, and the config directory only records which account to open it with. Given
a directory with no such record it uses the vault's token anyway, so the
directory alone is not a wall — see "Copilot's vault" below for what that
costs and the guard that answers it. When the vault cannot be reached the CLI
falls back to a plaintext file under that same directory, which is its own
behaviour, not something changed here.

`agent.MultiAccount` is the optional half of the provider contract for all
this: `DefaultHome`, `AccountStatus`, `LoginCommand`. A provider that does not
implement it offers the system account alone, and the API refuses to store a
second subscription for it rather than keep a path nothing will read.

## What a task remembers

`sessions.account_id`, `schedules.account_id` and `queued_messages.account_id`,
0 for the system account. A task keeps its subscription for the same reason it
keeps its provider: the opaque thread id belongs to the account that opened it,
and the other account cannot resume it. **Swapping subscription therefore
clears that id exactly as switching provider does**, and the next prompt starts
a new conversation.

A prompt waiting in the queue carries its own subscription, as it already
carried its own model, so one waiting prompt can be moved to another account
without touching the task's own choice. A job's runs inherit the job's.

Two rules follow from money rather than from code:

- **A removed subscription stops its tasks.** The row is deleted, the tasks
  keep pointing at it, and their next turn fails with
  `runner.ErrUnknownAccount`. Falling back to the machine's own login would
  quietly spend a personal allowance on work prompts, which is worse than not
  running them. Settings says how many tasks and jobs are affected before
  removing, and the login directory is left on disk.
- **A title request runs on the task's subscription.** Naming a task is a
  small charge, but it belongs on the account that asked for the work.

## What is isolated, and what a named account does not get

The whole point is that choosing a subscription here cannot disturb anything
outside this tool, including CLIs run by hand at the same time. It does not:

- The directory is set on the child process only — `cmd.Env` for the two
  environment-variable providers, an argument for Copilot. This process's own
  environment is never modified, so nothing leaks to the next turn or to
  anything else on the machine.
- Nothing here writes to a CLI's own configuration directory. The one write in
  the whole feature is `MkdirAll(<account home>, 0700)` when a login is
  launched, and the system account has no home to make. `UserHomeDir` is used
  only to *name* the default directory and to read a status out of it.
- Concurrency is the CLIs' own: each turn is a separate process with its own
  directory, so two subscriptions run side by side, and a CLI started in a
  terminal is unaffected. Two tasks both on the system account share that
  directory exactly as two hand-started CLIs already do.

Measured on the machine this was built on, rather than assumed:

| Check | Result |
|-------|--------|
| `CODEX_HOME=<fresh> codex login status` | `Not logged in`, while the real home says `Logged in using ChatGPT` |
| `CLAUDE_CONFIG_DIR=<fresh> claude -p "say ok"` | `Not logged in · Please run /login`, cost 0 |
| `copilot --config-dir <fresh>`, `account.getQuota` | **the machine's own quota**, identical figures — see below |

The other side of that isolation is worth saying plainly: **a named account
starts blank.** A fresh Claude Code directory has no `settings.json`, no user
memory, no agents, commands or plugins, and no MCP servers; a fresh
`CODEX_HOME` has no `config.toml` and no `~/.codex/AGENTS.md`; a fresh Copilot
directory has no `mcp-config.json` and no custom instructions. Only the system
account carries the machine-wide setup. That is the correct default for the
problem this solves — a work subscription should not inherit a personal
account's tools — and a subscription that wants some of it has to be given it,
by copying or linking the non-credential files into its directory.

## Copilot's vault, and the guard it needs

Asked about a directory nothing has signed into, the Copilot CLI does not
refuse: it finds the token in the machine's vault and answers for **that**
account. A fresh config directory reported the same quota as the machine's own
login, to the decimal. So for Copilot the directory selects the settings and
records which login to use, but it is not on its own a wall around the
account.

`runner.accountHome` therefore refuses a turn on any named subscription whose
directory holds no login, for every provider —
`runner.ErrAccountSignedOut`, a 400 saying which subscription to sign in.
Claude Code and Codex would refuse it themselves, so the rule costs them only
a clearer message; for Copilot it is the only thing preventing a task from
running on the account the machine already had. The allowance query is guarded
the same way: a subscription that is not signed in shows no bars rather than
another account's.

What this does **not** prove is that two genuinely signed-in Copilot accounts
stay apart — the vault keys its entries per account and `config.json` records
which one a directory uses, which is the CLI's own design, but verifying it
needs a second paid Copilot subscription and none was available. If it turns
out not to hold, the documented alternative is the environment token the CLI
checks first (`COPILOT_GITHUB_TOKEN`, then `GH_TOKEN`, then `GITHUB_TOKEN`),
which would be held as the *name* of a variable or a command to read it from,
never as a stored secret.

## Allowances

`Metered.SubscriptionLimits(ctx, home)` asks about one login, and remembered
readings are scoped to `(provider, account_id)` — two subscriptions have two
allowances, and a work account's bars drawn against a personal one would be
worse than no bars at all. The UI keys its cache by `provider:account`,
re-reads when the task's subscription changes, and names the account under the
plan in the panel. Everything else about the bars is [011](011-prompt-bar-usage.md).

## Signing in

Handed to a terminal, with the directory preset. All three CLIs draw their
login on a TTY and answer keypresses; probed through a pipe **and** through a
pseudo-terminal, none of them prints the URL or code that a browser flow would
need to relay. So there is nothing to relay: `openTerminal` asks the desktop
for a terminal the way `openInSystem` asks it for a file handler
([048](048-open-in-system-browser.md)), trying `x-terminal-emulator` and the
common desktops on Linux, Terminal.app on macOS, `cmd /k` on Windows, and the
shell line holds the window open after the CLI exits so a failure can be read.

The command is returned to the UI either way, ready to paste: a machine with no
terminal installed, or a browser reading the UI from another machine
([043](043-exposed-server.md)), is told exactly what to run. `claude /login`,
`codex login`, `copilot login --config-dir …`.

Sign-in status is read from the login directory itself — no CLI is started, so
the list is instant — and only non-secret fields are read: the plan name for
Claude Code, the auth mode for Codex, the GitHub login for Copilot. The tokens
in those same files are never decoded.

## The API

| Route | What it does |
|-------|--------------|
| `GET /api/providers` | each provider now carries `accounts` (system first) and `multi_account` |
| `POST /api/providers/:provider/accounts` | add one; only `alias` is required |
| `PATCH /api/providers/:provider/accounts/:account` | rename it or repoint it |
| `DELETE /api/providers/:provider/accounts/:account` | forget it |
| `GET /api/providers/:provider/accounts/:account/usage` | how many tasks and jobs still run on it |
| `POST /api/providers/:provider/accounts/:account/login` | open the CLI's login in a terminal; returns the command |
| `PUT /api/providers/:provider/account` | choose the default; `0` is the system account |
| `GET /api/providers/:provider/subscription-limits?account=N` | that subscription's allowance |

Every route taking both a provider and an account checks that the pair matches,
because a mismatch is a prompt run on the wrong allowance. An alias is unique
per provider and `System` is reserved. A home left out of a create is one the
app manages: `<data dir>/accounts/<provider>/<alias slug>`, created at 0700
when the login is launched, kept out of the CLI's own directory so removing or
signing out of this one can never touch the account the machine already had.

## On screen

- **Settings → Subscriptions.** One labelled group per provider: its rows, a
  `Make default` per row, `Sign in`, rename-and-repoint in one form, remove
  with its usage count, and `Add a … subscription` where more than one is
  possible. Each row shows its login folder and whether it is signed in.
- **Every model control.** The prompt bar's picker, a queued prompt's and a
  job's all draw one group per provider *and subscription* — `Claude Code ·
  Work` — with the alias in each entry's label and in its searchable
  description. A provider with one subscription keeps its plain name and its
  old layout. The chosen model reads back as `Opus · Work` on the button. All
  three build their groups from `modelPickerGroups` in the store and share one
  value format, `model:<provider>:<account>:<model>`.
- **Favourites carry no subscription.** A star is a model and an effort
  ([009](009-model-picker.md)); picking one keeps the task on the account it is
  already on, which is the account whose allowance is on screen beside it.
- **Stats** names the subscription under the provider, when there is more than
  one to name.

## Tested by

`e2e/tests/subscriptions.spec.js` drives the whole flow against the fake
provider ([005](005-testing.md)), which holds subscriptions the way the real
CLIs do: the system account alone until one is added, adding and renaming one,
moving the default, the alias appearing in the picker and on the button, and
removing one reporting what still runs on it. Two of its tests assert the
subscription a turn *actually* ran on, using the fake provider's `@account`
directive to report the login directory the runner resolved — the only way to
see that from a browser.

## Known gaps

Two subscriptions of one real Copilot account have never been run side by
side; see the section above for what was measured and what stands in for it.

A Copilot model list is per account: which models exist and which the account
is entitled to both come from the CLI ([036](036-copilot-model-list.md)), and
`Models()` has no account to ask about. So both subscriptions of one Copilot
account are offered the list the default login's CLI answered with, and a model
only the other account has — or lacks — is offered or hidden wrongly. Claude
Code and Codex are unaffected: their lists are static.
