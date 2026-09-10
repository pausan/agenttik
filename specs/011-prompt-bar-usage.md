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
shows one thin bar per allowance window in that shared click target — one per
window across every bucket the provider sends, because Codex packs its two
into a single bucket while Claude Code sends a bucket per window and a plan can
meter more than two. The panel reveals the current percent, exact window/reset
time, plan, and any reached-limit status. There are two ways a provider can
supply a reading:

- **Ask.** Codex answers `account/rateLimits/read` on its local app-server, so
  its figures are current on every request. `agent.Metered` is the optional
  half of the provider contract for this. Its stdin has to stay open until the
  answer arrives — app-server fetches over the network and exits the moment its
  input closes, which is why a write-then-close client gets no reply at all.
  Claude Code answers too, by being asked its own `/usage`.
- **Remember.** Claude Code also volunteers a reading mid-turn on a
  `rate_limit_event` line: one bucket — whichever is nearest its ceiling —
  with a `rateLimitType`, a `utilization` fraction, and a reset time. It
  arrives only once a bucket has crossed a warning threshold, so a reading is
  stored on the turn beside the context window and the newest one is served
  back. The panel says how old it is rather than presenting it as current, and
  the bar also moves live while the turn runs.

Neither path involves a credential, and neither estimates an allowance from
token totals. A provider with neither shows no allowance bars.

The event carries no window duration, so the bucket type names the bar
(`five_hour` → 5-hour, `seven_day` → Weekly, `overage` → Overage); an
unrecognised type is shown as it arrived rather than guessed at.

## Asking Claude Code for every window

The event alone is not enough to draw the 5-hour and weekly bars, and that is
not a bug in the reading: it names one bucket, and only once that bucket has
crossed a warning threshold. A subscription with room left in all of them
reports nothing, so the bars never appeared for most sessions.

`claude -p "/usage" --output-format json` answers the whole allowance locally.
It is worth doing on every request: no model runs, `total_cost_usd` is 0,
`num_turns` is 0, and it returns in about two seconds. The flags are the ones
a title request uses — `--no-session-persistence --safe-mode --setting-sources
user --tools ""` — so no project settings or hooks load, no tools are
available, and no session is left behind. Slash commands stay enabled, since
`/usage` is the whole request. As with app-server, the CLI signs the call with
its own credential and agenttik never sees one.

The report is prose, one line per window:

```
Current session: 56% used · resets Sep 10, 8:59am (Europe/Madrid)
Current week (all models): 47% used · resets Sep 15, 3:59pm (Europe/Madrid)
Current week (Fable): 22% used · resets Sep 15, 3:59pm (Europe/Madrid)
```

Three details make parsing it safe rather than lucky:

- The percent is already 0–100 here, where the event's `utilization` is a
  fraction. Reading one as the other is the difference between 56% and 0.56%.
- The headings map onto the ids the event uses (`Current session` → `five_hour`,
  `Current week (all models)` → `seven_day`), so a mid-turn reading refreshes
  the bar `/usage` drew instead of adding a second one for the same window.
  Per-model weeklies come and go with the plans on offer — `Current week
  (Fable)` appeared without a release note — so an unlisted heading is named
  from its own title rather than dropped.
- The reset has no year, and the minutes vanish on the hour. Neither is a
  guess to resolve: the CLI names the zone in the text, prints the year
  precisely when it differs from the current one, and omits `:00`. Four
  layouts cover it, and an unreadable reset is left at 0, which the panel
  already shows as "not reported".

Because the mid-turn event names one bucket, it now updates that bar by id and
leaves the other windows standing; replacing the whole set would blank the
5-hour and weekly bars the moment an overage warning arrived.

Having both paths also means neither is a single point of failure. The ask
comes first, and the remembered reading stands in whenever it fails or comes
back empty — an unreachable CLI, or a report worded in a way the parser no
longer recognises, then shows the last known bars instead of none.

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

Against CLI 2.1.227, `/api/providers/claude/subscription-limits` answers
`five_hour` 57%, `seven_day` 47% and `seven_day_fable` 22%; the two parsed
resets match what the CLI printed, to the minute, once converted back to
Europe/Madrid. Codex still answers one `codex` bucket with both windows, so
its two bars are unchanged. Headless in the running app: three tracks with
widths 56%/47%/22%, a panel reading "5-hour · Weekly · Weekly (Fable)" with
each reset, and no console errors.
