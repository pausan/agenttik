# Session queue

## Outcome

A prompt can be queued with the Send menu or Ctrl+Enter (rebindable — see
[015-settings-shortcuts.md](015-settings-shortcuts.md)). Queued prompts are persisted and marked with a clock in session lists.

Each project runs queued prompts only after its active turns finish. The scheduler keeps draining the session that just ran while it has queued prompts; otherwise it scans the projects open sessions from top to bottom.

A queued prompt is visible where it will run: the transcript draws one dashed
bubble per waiting prompt under the working indicator, each with a timer of how
long it has waited. Without it, clicking into a session said nothing about the
text that had been accepted.

Every waiting prompt also offers to run now, from the conversation it belongs
to. What that costs depends on what the prompt is waiting behind, and the two
words say which:

- **Send now**, when the session itself is idle. The prompt is held only by the
  project's ordering rule, not by a process it has to share, so it starts
  *beside* whatever else the project is running and interrupts nothing. This is
  the common case — the turn in the way usually belongs to a *different*
  session, which is what queueing behind a project means, and cancelling a
  stranger's work to jump a queue is a price nobody asked to pay.
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

The sidebar keeps its own **Stop** for a queued session ([021](021-stop-active-sessions.md)); running now
and dropping the queue are different intents and both are worth one click.

Session detail carries the queue, so opening a session or reloading the page
shows it. The POST that queues a prompt answers with the queue *after*
scheduling, so a prompt the scheduler took straight away is never drawn as
waiting. The turn that claims a prompt moves its text out of the queued bubble
and into the transcript, and the end of every turn re-reads the queue from the
store, which reconciles the same text queued twice.

The prompt box is also cleared before the request rather than after it, as
sending already did. Queueing two prompts in a row lost the second one: the
answer to the first arrived while it was being typed and blanked the box.

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

`make ui`, `go vet ./...` and the web unit tests (32) passed. `go test ./...`
fails only the two cases that already failed before this work.

Browser-checked headlessly against a fake `claude` on PATH that sleeps, so a
turn stays running and the scheduler leaves the queue alone: two prompts queued
back to back both appear, oldest first, their timers tick, they survive a
reload, and stopping the turn moves the first into the transcript as a prompt
while the second keeps waiting. No console or page errors.

## Force-send validation

`go build ./...` and `make ui` passed. Tests were not run, per the prototype
workflow. Force send cancels the active turn, then the runner claims the chosen
queued row before normal scheduling resumes.

## Send-now validation

`make ui`, `go build ./...`, `go vet ./...` and the web unit tests (33) passed.

Browser-checked headlessly with a fake `claude` that sleeps: session A runs a
turn while session B queues two prompts in the same project. B is not running,
so both its bubbles read **Send now**. Clicking the *second* one cancels A's
turn (superseded — see below; it no longer does), starts B on that prompt, and
leaves the first still queued — the forced prompt really does jump the order. B's remaining bubble then reads **Force
send**, since B is the running session now. No console or page errors.

Fixed on the way: `api()` parsed the body of a bare `202 Accepted` as JSON, so
a *successful* force raised a "Something went wrong" toast. It now decodes only
a JSON content type; failures are always JSON.

## Per-item model validation

`go build ./...` and `npm run build` passed. Tests were not run, per the
prototype workflow.

## Parallel send-now validation

Forcing used to be a project-wide interruption: **Send now** cancelled every
turn in the project, so a prompt queued in an idle session killed the turn of
whatever else was running — including a scheduled job's run
([028](028-scheduled-jobs.md)), whose session is an ordinary one. Waiting for a
stranger to be cancelled was never the point of the button; jumping the queue
was.

`go build ./...`, `go vet ./...` and `make ui` pass.

Probed against the running app with a fake `claude` that sleeps:

- Session A running, a prompt queued in idle session B: **Send now** left A
  running and started B beside it. Two provider processes, both sessions
  `running`, B's queue empty.
- A prompt queued in B while B itself runs: **Force send** replaced B's turn
  with it and left A's turn alone — A's transcript still held only its
  original prompt.
- A schedule's run in flight, **Send now** in another session: the run survived
  and was still `running` afterwards, which is the case that was reported.
