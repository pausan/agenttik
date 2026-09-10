# The task queue

A prompt can be queued with the Send menu or Enter (rebindable — see
[015](015-settings-shortcuts.md)). Queued prompts are persisted and marked with
a clock in the task lists.

Each project runs queued prompts only after its active turns finish. The
scheduler keeps draining the task that just ran while it has queued prompts;
otherwise it scans the project's open tasks from top to bottom.

A queued prompt is visible where it will run: the transcript draws one dashed
bubble per waiting prompt under the working indicator, each with a timer of how
long it has waited, so clicking into a task shows the text it has accepted
rather than nothing.

## Running a waiting prompt now

Every waiting prompt offers to run now, from the task it belongs to.
What that costs depends on what the prompt is waiting behind, and the two words
say which:

- **Send now**, when the task itself is idle. The prompt is held only by the
  project's ordering rule, not by a process it has to share, so it starts
  *beside* whatever else the project is running and interrupts nothing. This is
  the common case — the turn in the way usually belongs to a *different* task,
  which is what queueing behind a project means, and cancelling a stranger's
  work to jump a queue is a price nobody asked to pay. A scheduled job's run
  ([028](028-scheduled-jobs.md)) is an ordinary task, so this is what keeps it
  out of the blast radius.
- **Force send**, when the task is running its own turn. A provider takes one
  prompt at a time, so that turn really is in the way: it is cancelled and the
  prompt starts as it exits. Only that task's turn — the rest of the project
  keeps going.

Either way the prompt bypasses ordering exactly once; the queue behind it
resumes its ordinary scheduler order. The selected prompt remains queued until
its replacement turn starts, which keeps it visible if cancellation or startup
fails.

The runner holds one forced prompt per *task*, claimed by that task's turn as
it releases, and the project dispatch lock keeps it from racing the scheduler
draining the same project.

The sidebar keeps its own **Stop** for a queued task
([021](021-stop-active-tasks.md)); running now and dropping the queue are
different intents and both are worth one click.

## Reconciling the queue

`GET /api/sessions/:id` carries the queue, so opening a task or reloading the
page shows it. The POST that queues a prompt answers with the queue *after*
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
change the task's default or other queued items.

When the runner claims an item, it applies that saved choice immediately before
starting the turn. A provider change clears the provider-owned thread id; a
model or effort change within the current provider keeps it.

A prompt whose turn failed because the provider was away goes back in this same
queue with a clock on it; see [045](045-provider-outage-retry.md).
