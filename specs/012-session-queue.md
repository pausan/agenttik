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
to. A session that is running its own turn calls it **Force send**; a session
that is only waiting in the project queue calls it **Send now**. Both do the
same thing: cancel the turns the project has in flight, then start the chosen
prompt when the last of them exits. A running CLI cannot safely accept another
prompt in place, so this is deliberately an interruption rather than an in-turn
message. The selected prompt remains queued until its replacement turn starts,
which keeps it visible if cancellation or startup fails.

The interruption is a project-wide intent rather than a note on one running
session, because the turn in the way usually belongs to a *different* session:
that is what queueing behind a project means. The runner holds one forced
prompt per project, whichever cancelled turn exits last claims it, and the
project dispatch lock keeps it from racing the scheduler that is draining the
same project. Nothing to interrupt means the prompt starts straight away
rather than waiting for a turn that is not coming.

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
turn, starts B on that prompt, and leaves the first still queued — the forced
prompt really does jump the order. B's remaining bubble then reads **Force
send**, since B is the running session now. No console or page errors.

Fixed on the way: `api()` parsed the body of a bare `202 Accepted` as JSON, so
a *successful* force raised a "Something went wrong" toast. It now decodes only
a JSON content type; failures are always JSON.

## Per-item model validation

`go build ./...` and `npm run build` passed. Tests were not run, per the
prototype workflow.
