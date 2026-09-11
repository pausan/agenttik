# Copilot model list

The GitHub Copilot provider no longer carries a hand-written model list. It
asks the CLI, which is the only authority worth asking: which models exist,
and which the signed-in account is entitled to, both change without a release
here.

The hard-coded list had gone stale in both directions. Of its 15 entries only
two — `gpt-5.3-codex` and `gpt-4.1` — are still served. It offered 13 ids the
CLI no longer has: `claude-sonnet-4.6`, `claude-sonnet-4.5`,
`claude-haiku-4.5`, `claude-opus-4.6`, `claude-opus-4.6-fast`,
`claude-opus-4.5`, `claude-sonnet-4`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.2`,
`gpt-5.2-codex`, `gpt-5.1`, `gpt-5-mini`. And it hid the 10 it does:
`claude-opus-4.7`, `claude-opus-4.8`, `claude-opus-5`, `claude-sonnet-5`,
`gemini-3.7-flash`, `gpt-5.6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`,
`kimi-k3`, `mai-code-1.1-flash`.

`SmallModel` — then called `TitleModel` — named `gpt-5.4-mini`, one of the ids
that had gone, so every Copilot session title was requested against a model the
CLI would refuse.

Three things now come from the CLI per model rather than from nothing:

- the context window, which the prompt bar's gauge measures against
- the reasoning levels the model accepts, which are narrower than the levels
  the model is capable of — the CLI restricts what it passes on, and reports
  only `medium` for the Claude models it serves
- the title model, picked as the cheapest the account can use from the prices
  in the same answer, so it cannot name a model that has been dropped

## Implementation

`models.list` on the same headless CLI server that `account.getQuota` already
used, so `limits.go`'s transport was extracted into `serverQuery`. Both are
read-only: no session is created and no model request is sent.

The response is already filtered for the caller — the CLI applies its own
`model_picker_enabled`/`policy.state` test before answering — so the list is
taken as given, in its order. The first entry is what a new session starts on,
which is the CLI's own first offer.

Asking costs a CLI start, measured at 1.9-2.8s, almost all of it process
startup. So:

- the answer is cached for 10 minutes, and `main.go` asks once at startup, in
  the background, so the UI's first request finds it already there
- the timestamp is taken before the ask, not after, so a CLI that is missing
  or logged out is retried on that same interval instead of on every request
- the cache lock is held across the ask, so a burst of requests starts one
  CLI and the rest read what it brought back
- `Available()` is checked first, so no CLI on PATH costs nothing at all

`Models()` stays synchronous and cannot fail, so nothing above it changed. A
fallback list stands in until the first answer arrives and for good when the
CLI cannot be asked: `Models()[0].ID` is the default model for a new session,
so the list must never be empty. That fallback is a snapshot and will go stale
the way the old list did — it exists so the app always has a model to name,
never as the list of record.

## Known gap

A model the CLI says takes no reasoning effort at all — `gpt-4.1`,
`gemini-3.7-flash`, `kimi-k3`, `mai-code-1.1-flash` — still shows the
provider-wide levels in the picker. `agent.Model.Efforts` is `omitempty`, so
an empty list is indistinguishable from an absent one on the wire, and the UI
reads absent as "use the provider's list". This is what every Copilot model
did before, so it is not a regression, but it does offer four models levels
the CLI would reject.
