# GitHub Copilot subscription

GitHub Copilot is the third provider in the registry. It runs through the
locally installed `copilot` CLI, streams its JSONL events into the existing
transcript, resumes the CLI session id, and reports token and context usage.
Its model list is asked of the CLI rather than written down; see
[036](036-copilot-model-list.md).

The provider also implements `agent.Metered`. The server starts a short-lived
headless Copilot CLI process and sends the read-only `account.getQuota` RPC
through the shared `serverQuery` transport, which the model list uses too —
see 036.
Each non-empty quota snapshot becomes one subscription bar. The known buckets
are `premium_interactions`, `chat`, and `completions`; unknown bucket ids are
shown with a readable title. Copilot's remaining percentage is converted to
the shared used percentage, and its ISO reset date is converted to Unix
seconds for the prompt bar.

Zero-entitlement empty buckets are omitted so a plan does not show bars for
allowances it does not have.

## Session contract

Prompt mode uses the CLI's non-interactive JSON stream:

```
copilot -p <prompt> --output-format json --stream on --model <model> \
        --effort <effort> --allow-all-tools --add-dir <workdir>
```

`session.start` supplies the provider session id. Assistant message and
reasoning deltas become transcript text, tool execution events become the
existing tool rows, `session.usage_info` supplies live context usage,
`assistant.usage` supplies turn totals and any volunteered quota snapshots,
and `session.idle` closes the turn. A cancelled turn terminates the CLI's
process group, matching the other providers.

The app's permission modes map to `--plan`, `--allow-all-tools`, and
`--allow-all`. Isolated title requests disable custom instructions and built-in
MCP servers.

## Authentication boundary

More than one account is selected with `--config-dir`, which every command
here carries: the token itself is in the machine's own vault, keyed per
account, and `<dir>/config.json` records which one to open it with. See
[050](050-subscription-accounts.md).

agenttik does not read VS Code extension storage, GitHub tokens, or Copilot
credentials. The installed `copilot` CLI must already be authenticated with
the GitHub account that owns the Copilot subscription. A machine with only the
VS Code extension and no `copilot` CLI reports the provider as unavailable.
