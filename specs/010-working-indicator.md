# Working indicator

While the current task has a running turn, the transcript ends with a
clock-face progress line and elapsed seconds. It uses the server turn's start
time when available, with a local timestamp only for the short gap after send.
The clock refreshes at 4 FPS, while the elapsed label remains rounded down to
whole seconds. The line says "Working", or what the turn waits on: an approval or
an answer. A turn that takes prompts also names its background tasks: "Waiting
for 2 background tasks · you can keep writing" between replies, "Working · 2
background tasks · you can keep writing" while the agent replies (see
[083](083-background-tasks.md)). The line is tied to the current session and disappears as soon
as its turn is done or stopped.

The provider's completion event and the persisted completion event are both
received. Only the persisted one, which carries the final stats, is
deduplicated and clears the progress line; a turn that waits on background
tasks sends a provider completion event for each of its replies.

Project, task, and profile activity dots pulse in opacity and size together over
two seconds: full opacity at their existing size, fading to half opacity at 60%
size, then growing again. Scaling does not change layout. Waiting dots use the
same animation over 2.5 seconds. The pulse moves in steps, about four a
second, rather than every frame (see [079](079-ui-performance.md)).

## Turn summary

When a turn finishes, the progress line gives way to one dimmed line under the
turn's last message. It ends with the clock time the turn finished. From the
second turn on, it also carries the task's totals up to that turn, so the
figures grow down the transcript and the last line always holds the whole task:
`2min · 300k in / 5k out · $0.40 · 12:43:42 — total 1h 3min · 1.3M in / 29k out · $3.22`.
It is a glance, not an account — Stats has the exact figures:

- Time is exact below two minutes (`42s`, `1min 35s`) and rounded to minutes
  after (`14min`, `1h 3min`).
- Tokens are the turn's reported input and output, rounded (`850`, `2.4k`,
  `29k`, `1.3M`).
- Cost is the reported cost plus any fallback estimate, in dollars to the
  cent, `≈` when an estimate contributes and `<$0.01` below a cent. It is left
  off when the turn reported no cost.
- The finish time is a 24-hour clock with seconds, like message labels.

The totals add every finished turn up to this one, including turns whose
messages an edit removed, as Stats does. Time is the sum of turn durations,
not wall-clock time since the first prompt.

The line is placed from each message's `turn_id`. Prose and tool messages
streamed during a turn have none; the transcript is re-read when the turn
ends, which supplies them. Background task events carry stored messages with
turn ids. `turnSummaries` in `web/src/api.js` formats the line.
