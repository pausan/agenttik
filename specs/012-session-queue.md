# Session queue

## Outcome

A prompt can be queued with the Send menu or Ctrl+Enter (rebindable — see
[015-settings-shortcuts.md](015-settings-shortcuts.md)). Queued prompts are persisted and marked with a clock in session lists.

Each project runs queued prompts only after its active turns finish. The scheduler keeps draining the session that just ran while it has queued prompts; otherwise it scans the projects open sessions from top to bottom.

A queued prompt is visible where it will run: the transcript draws one dashed
bubble per waiting prompt under the working indicator, each with a timer of how
long it has waited. Without it, clicking into a session said nothing about the
text that had been accepted.

While that same session is active, each waiting prompt also offers **Force
send**. It stops the current turn, then runs the selected prompt next in that
provider conversation. A running CLI cannot safely accept another prompt in
place, so this is deliberately an interruption rather than an in-turn message.
The selected prompt remains queued until its replacement turn starts, which
keeps it visible if cancellation or startup fails.

Session detail carries the queue, so opening a session or reloading the page
shows it. The POST that queues a prompt answers with the queue *after*
scheduling, so a prompt the scheduler took straight away is never drawn as
waiting. The turn that claims a prompt moves its text out of the queued bubble
and into the transcript, and the end of every turn re-reads the queue from the
store, which reconciles the same text queued twice.

The prompt box is also cleared before the request rather than after it, as
sending already did. Queueing two prompts in a row lost the second one: the
answer to the first arrived while it was being typed and blanked the box.

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
