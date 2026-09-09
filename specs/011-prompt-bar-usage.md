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

When the selected provider reports subscription buckets, the prompt bar also
shows one thin bar per allowance window in that shared click target. The panel
reveals the current percent, exact window/reset time, plan, and any reached-limit
status. Codex uses its local app-server's read-only account/rateLimits/read
method; no credentials or estimated usage are stored by agenttik. Providers
without that method show no allowance bars.

## Validation

go build ./... and npm run build passed. A live Claude Code turn through the
running app stored the reported window (haiku, 200k) and the panel read
16.6k / 200.0k; a CLI probe confirmed `claude-opus-5` reports a 1M window,
which the old 200k constant had been clamping to a permanent 100%. The
migration was applied against a copy of the real database. No tests were run.
