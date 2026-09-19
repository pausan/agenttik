# Subscriptions

A subscription is an alias and a login directory for one provider. Every task,
queued prompt and job names the subscription it uses. Settings adds and signs
in accounts; every model picker names the selected subscription.

Claude Code, Codex and Copilot authenticate through their CLIs. OpenCode Go
also supports direct use with its API key; its policy, storage and execution
contract are in [065](065-opencode-subscription.md).

## Accounts and isolation

**System**, account `0`, has no database row. It uses the provider's default
login folder and cannot be moved or removed. Its friendly name can be edited
in Settings, with a persistent **system** badge identifying it. Names are trimmed,
nonempty, at most 80 bytes, and cannot match a named subscription. It can be signed
in or replaced as the default by a named account. Subscriptions, defaults and
System names are shared across local profiles; model choices and favourites are
per profile.

| Provider | Account selection | Login location |
|---|---|---|
| Claude Code | `CLAUDE_CONFIG_DIR` | `<dir>/.credentials.json` |
| Codex | `CODEX_HOME` | `<dir>/auth.json` |
| GitHub Copilot | `--config-dir` | OS vault; `<dir>/config.json` identifies the account |
| OpenCode Go | `XDG_DATA_HOME` for CLI; direct file lookup | `<dir>/opencode/auth.json` |

`agent.MultiAccount` provides `DefaultHome`, `AccountStatus`, `LoginCommand`.
Providers without it offer only System. A named account with no home supplied
uses `<data dir>/accounts/<provider>/<alias slug>`. Directories created for
login use 0700. Removing a subscription leaves its login files on disk.
Environment overrides are child-local; no login changes the parent process.
Named CLI folders start with no inherited settings, memories, plugins or MCP
configuration. Users can copy/link non-credential configuration when desired.

A task, queued message or schedule stores `account_id`. Changing account
clears its provider conversation id, as changing provider does. Title and
outcome requests spend the task's account. Removed accounts stop their tasks
with `runner.ErrUnknownAccount`; there is no fallback to System.

Named accounts must pass `AccountStatus` before a turn or allowance query.
This is necessary for Copilot: an empty config directory can otherwise use the
machine vault's existing token. A named account with no login produces
`runner.ErrAccountSignedOut`, rather than using someone else's allowance.
Two independently paid Copilot accounts have not been tested side by side.
The CLI's vault selection remains responsible for separation after login.

## Signing in and choosing execution

Each provider group explains whether its CLI must be installed, includes an
installation link, and reports detection on the server's PATH. Claude is
CLI-only. Codex and Copilot use documented CLI-backed subscription integration.
OpenCode's CLI is optional and its account form offers Automatic, Direct and
CLI; automatic chooses an installed CLI for new conversations.

CLI login opens a desktop terminal with the account directory preset and
returns the shell command for manual use. Failure to find a desktop terminal
still leaves a copyable command. Remote browser users run it on the server.
Claude uses `claude /login`, Codex `codex login`, Copilot its configured login
command. Login status is read from non-secret metadata for those providers.

OpenCode's settings form accepts a Go key from its console without a CLI.
`agent.DirectAccount` supplies connection preferences and key storage; only
providers implementing it accept direct-login API requests. Key input is
cleared after saving. No saved key is sent back to the UI. OpenCode reads its
own API key for both presence and direct requests; other providers' tokens
are never extracted. The terminal-login option is also available when the
OpenCode CLI is installed.

## Allowances and model pickers

`Metered.SubscriptionLimits(ctx, home)` asks about one login. Remembered
readings and browser caches are keyed by `(provider, account_id)` so two
subscriptions never display each other's allowances. Signed-out named
accounts have no bars. See [011](011-prompt-bar-usage.md).

Every picker uses `modelPickerGroups` and values in the form
`model:<provider>:<account>:<model>`. Entries and buttons include the account
alias. Favourites include subscription, provider, model and effort. Task Stats
also names the account when more than one exists. Automatic task selection
skips a direct provider until its default account has a saved key.

Copilot's model catalogue still uses the default CLI login, so model
entitlements specific to another named account may be shown incorrectly.
Claude Code and Codex use static lists. OpenCode offers its adapter's supported
Go models; see [065](065-opencode-subscription.md).

## API

| Route | Purpose |
|---|---|
| `GET /api/providers` | Accounts, multi-account support, CLI detection/requirements and direct-login capability |
| `POST /api/providers/:provider/accounts` | Add; alias required, home optional |
| `PATCH /api/providers/:provider/accounts/:account` | Rename or repoint |
| `DELETE /api/providers/:provider/accounts/:account` | Forget, retaining login files |
| `GET /api/providers/:provider/accounts/:account/usage` | Affected task/job counts |
| `POST /api/providers/:provider/accounts/:account/login` | Open CLI login; return shell command |
| `PUT /api/providers/:provider/accounts/:account/connection` | Save `connection` (`auto`, `direct`, `cli`) and optional write-only `key` |
| `PUT /api/providers/:provider/account` | Select default account |
| `GET /api/providers/:provider/subscription-limits?account=N` | Account allowance |

Provider/account pairs are validated on every account route. Aliases are
unique per provider; System is reserved. Account responses include a
`connection` preference only for direct-capable providers. Unsupported direct
login, including Claude, is refused by the server.

## Tests

Server/account tests cover defaults, CRUD, mismatched providers, login
isolation, direct-login capability and key non-disclosure. The subscription
browser suite covers adding, renaming, default selection, picker aliases,
actual fake-provider turn account selection, removal consequences, and
OpenCode's terminal-free connection form. OpenCode provider tests exercise
both execution paths without a live paid subscription.
