# Prompt-bar usage indicators

## Outcome

The model control is a fuzzy-search palette. It preserves provider groups and
favourite model-and-effort combinations, and searching matches both the display
name and model id.

The prompt bar shows context usage as a circular ring. Clicking the ring or an
allowance bar opens one panel: context count, window, and session totals are on
the left; subscription details are on the right.

The window the ring measures against is the one the provider reports, not a
per-model constant. Claude Code names it on every result line
(`modelUsage[model].contextWindow`); the newest turn that reported one is
stored on the turn and preferred by the gauge, so a model alias run in its 1M
variant reads correctly instead of pinning the ring at 100%. The static
per-model figure is only what to show before the first turn finishes.

A turn routinely runs more than one model — auto mode classifies with a small
one, a subagent can use another again — so the breakdown is read against the
alias the session asked for rather than by taking the largest window in it. A
haiku turn that also touched Sonnet reports both 200k and 1M, and measuring
23.6k of context against the wrong one of those is the difference between 12%
and 2%. The largest window stays the fallback for an alias that matches
nothing, so an unrecognised name never shrinks the gauge below the real
ceiling.

When the selected provider reports subscription buckets, the prompt bar also
shows one thin bar per allowance window in that shared click target. The panel
reveals the current percent, exact window/reset time, plan, and any
reached-limit status. There are two ways a provider can supply one, and each
has exactly one:

- **Ask.** Codex answers `account/rateLimits/read` on its local app-server, so
  its figures are current on every request. `agent.Metered` is the optional
  half of the provider contract for this. Its stdin has to stay open until the
  answer arrives — app-server fetches over the network and exits the moment its
  input closes, which is why a write-then-close client gets no reply at all.
- **Remember.** Claude Code never answers a query, but it volunteers a reading
  mid-turn on a `rate_limit_event` line: one bucket — whichever is nearest its
  ceiling — with a `rateLimitType`, a `utilization` fraction, and a reset time.
  It arrives only once a bucket has crossed a warning threshold, so a reading
  is stored on the turn beside the context window and the newest one is served
  back. The panel says how old it is rather than presenting it as current, and
  the bar also moves live while the turn runs.

Neither path involves a credential, and neither estimates an allowance from
token totals. A provider with neither shows no allowance bars.

The event carries no window duration, so the bucket type names the bar
(`five_hour` → 5-hour, `seven_day` → Weekly, `overage` → Overage); an
unrecognised type is shown as it arrived rather than guessed at.

## Validation

`go build ./...`, `go vet ./...` and `npm run build` pass. A live Claude Code
haiku turn through the running app stored `context_window` 200000 and
`rate_limits` `[{"limit_id":"overage","primary":{"used_percent":88,…}}]`, and
`GET /api/providers/claude/subscription-limits` served it back with
`reported_at`. The same turn before the per-model change stored 1000000,
because its breakdown also carried a Sonnet entry from auto mode. Codex's
app-server path still answers in 0.9s. The migration was applied against a
copy of the real database. `go test ./app/internal/agent/...` passes;
`TestDoneReachesProjectTopic` and `TestReorderSessionsDrivesProjectOrder`
already failed before this change and still do.
