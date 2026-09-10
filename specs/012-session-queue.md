# Session queue

## Outcome

A prompt can be queued with the Send menu or Enter (rebindable — see
[015-settings-shortcuts.md](015-settings-shortcuts.md)). Queued prompts are
persisted and marked with a clock in session lists.

Each project runs queued prompts only after its active turns finish. The
scheduler keeps draining the session that just ran while it has queued prompts;
otherwise it scans the project's open sessions from top to bottom.

A queued prompt is visible where it will run: the transcript draws one dashed
bubble per waiting prompt under the working indicator, each with a timer of how
long it has waited, so clicking into a session shows the text it has accepted
rather than nothing.

## Running a waiting prompt now

Every waiting prompt offers to run now, from the conversation it belongs to.
What that costs depends on what the prompt is waiting behind, and the two words
say which:

- **Send now**, when the session itself is idle. The prompt is held only by the
  project's ordering rule, not by a process it has to share, so it starts
  *beside* whatever else the project is running and interrupts nothing. This is
  the common case — the turn in the way usually belongs to a *different*
  session, which is what queueing behind a project means, and cancelling a
  stranger's work to jump a queue is a price nobody asked to pay. A scheduled
  job's run ([028](028-scheduled-jobs.md)) is an ordinary session, so this is
  what keeps it out of the blast radius.
- **Force send**, when the session is running its own turn. A provider takes
  one prompt at a time, so that turn really is in the way: it is cancelled and
  the prompt starts as it exits. Only that session's turn — the rest of the
  project keeps going.

Either way the prompt bypasses ordering exactly once; the queue behind it
resumes its ordinary scheduler order. The selected prompt remains queued until
its replacement turn starts, which keeps it visible if cancellation or startup
fails.

The runner holds one forced prompt per *session*, claimed by that session's
turn as it releases, and the project dispatch lock keeps it from racing the
scheduler draining the same project.

The sidebar keeps its own **Stop** for a queued session
([021](021-stop-active-sessions.md)); running now and dropping the queue are
different intents and both are worth one click.

## Reconciling the queue

Session detail carries the queue, so opening a session or reloading the page
shows it. The POST that queues a prompt answers with the queue *after*
scheduling, so a prompt the scheduler took straight away is never drawn as
waiting. The turn that claims a prompt moves its text out of the queued bubble
and into the transcript, and the end of every turn re-reads the queue from the
store, which reconciles the same text queued twice.

The prompt box is cleared before the request rather than after it: the answer
to one prompt arriving while the next is being typed would otherwise blank the
box and lose it.

## Per-item model choices

Each queued prompt records the provider, model and effort selected when it was
enqueued. Its dashed transcript bubble shows the model and offers the same
cross-provider model search plus an effort picker. Editing one item does not
change the session default or other queued items.

When the runner claims an item, it applies that saved choice immediately before
starting the turn. A provider change clears the provider-owned thread id; a
model or effort change within the current provider keeps it. Existing queued
rows are populated from their session during migration.

## Validation

`go build ./...`, `go vet ./...` and `make ui` pass. `go test ./...` fails only
the two cases listed in [index.md](index.md), which are unrelated to the queue.

Checked against the running app with a fake `claude` on PATH that sleeps, so a
turn stays running and the scheduler leaves the queue alone:

- Two prompts queued back to back both appear, oldest first, their timers tick,
  and they survive a reload. Stopping the turn moves the first into the
  transcript as a prompt while the second keeps waiting.
- Session A running, a prompt queued in idle session B: **Send now** leaves A
  running and starts B beside it. Two provider processes, both sessions
  `running`, B's queue empty.
- A prompt queued in B while B itself runs: **Force send** replaces B's turn
  with it and leaves A's turn alone.
- A schedule's run in flight, **Send now** in another session: the run survives
  and is still `running` afterwards.
- The bubbles read what they do: **Force send** in the running session,
  **Send now** in the idle one.

No console or page errors throughout.
