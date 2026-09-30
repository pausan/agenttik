# Working indicator

While the current task has a running turn, the transcript ends with a
clock-face progress line and elapsed seconds. It uses the server turn's start
time when available, with a local timestamp only for the short gap after send.
The clock refreshes at 4 FPS, while the elapsed label remains rounded down to
whole seconds. The line is tied to the current session and disappears as soon
as its turn is done or stopped.

The provider's completion event and the persisted completion event are both
received. They are deduplicated independently, so the persisted event carrying
the final stats always clears the progress line.

Project, task, and profile activity dots pulse in opacity and size together over
two seconds: full opacity at their existing size, fading to half opacity at 60%
size, then growing again. Scaling does not change layout. Waiting dots use the
same animation over 2.5 seconds.

## Turn summary

When a turn finishes, the progress line gives way to one dimmed line under the
turn's last message: `1h 3min · 1.3M in / 29k out · $3.22`. It is a glance,
not an account — Stats has the exact figures:

- Time is exact below two minutes (`42s`, `1min 35s`) and rounded to minutes
  after (`14min`, `1h 3min`).
- Tokens are the turn's reported input and output, rounded (`850`, `2.4k`,
  `29k`, `1.3M`).
- Cost is the reported cost plus any fallback estimate, in dollars to the
  cent, `≈` when an estimate contributes and `<$0.01` below a cent. It is left
  off when the turn reported no cost.

The line is placed from each message's `turn_id`. Messages streamed during a
turn have none; the transcript is re-read when the turn ends, which supplies
them. `turnSummary` in `web/src/api.js` formats it.
