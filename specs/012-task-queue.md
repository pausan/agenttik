# The task queue

A prompt can be queued with the Send menu or Enter (rebindable — see
[015](015-settings-shortcuts.md)). Queued prompts are persisted and marked with
a clock in the task lists.

Each project runs queued prompts only after its active turns finish. The
current task keeps priority while it has queued prompts, even when tasks
above it have waiting prompts or the user reorders the list. Prompts added to
that task while it runs keep the same priority. Within a task, prompts run in
acceptance order. Once its queue drains, the scheduler picks the next task
with ready prompts from the project's open tasks, top to bottom in the sidebar.

The runner remembers the task that most recently finished a turn for every
project. All automatic dispatch uses that preference, including new enqueues,
schedule and retry ticks, and Resume after a pause. Competing dispatch calls
cannot lose it between turns. A deleted task or one without queued prompts is
skipped. A provider retry holds the entire project's automatic queue, even
for other models, until the current task can continue
([045](045-provider-outage-retry.md)). Running and explicitly forced turns
remain allowed; other projects keep running.
The ordinary preference lives in memory: after an app restart, selection starts
at the top of the project list. A saved provider retry keeps its priority across
restarts and parallel task completions.

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
- **Send now** again when that turn only waits on background tasks and takes
  prompts ([083](083-background-tasks.md)): the prompt joins the turn and
  nothing is cancelled. The request then answers at once with
  `{"joined": true, "queued": [...]}`, and the window updates only the queue.

Either way the prompt bypasses ordering exactly once; the queue behind it
resumes its ordinary scheduler order. The selected prompt remains queued until
its replacement turn starts, which keeps it visible if cancellation or startup
fails.

The runner holds one forced prompt per *task*, claimed by that task's turn as
it releases, and the project dispatch lock keeps it from racing the scheduler
draining the same project.

`POST /api/sessions/:id/queue/force` answers once the prompt has started, or
with the reason it could not (a failed launch, a prompt deleted meanwhile).
It waits at most 30 seconds for a turn to stop; past that it answers `202` and
the prompt starts on its own. The UI then reads the session back before the
button stops spinning, so the button never returns to idle between the answer
and the new turn reaching the screen. That gap is seconds long over a remote
link. The button keeps the word that was pressed while it spins, even though
the cancelled turn's end would otherwise turn "Force send" into "Send now".

The sidebar keeps its own **Stop** for a queued task
([021](021-stop-active-tasks.md)); running now and dropping the queue are
different intents and both are worth one click.

## Deleting a queued item

Each waiting bubble has a trash button that deletes just that prompt. It
leaves the running turn, the remaining queue and the prompt-box draft alone.
Deleted text is removed from the queue rather than saved in the transcript.
The button is disabled until the server has accepted the prompt and while
another queue action is in progress. A failed deletion keeps the bubble and
shows the error.

`DELETE /api/sessions/:id/queue/:queuedID` checks that the item belongs to the
session and holds the project's dispatch lock while removing it. An item
already claimed by a turn returns 404. Queue counts and other windows refresh
through the usual session-change event. Removing the last provider retry
returns a waiting task to idle; removing the last prompt from an unstarted
scheduled run closes that run as interrupted without spending its counter.
Deleting a held retry also reschedules the project so other queued work can
continue if no retry remains.

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

## Editing a queued item

A queued prompt has not started, so its text is as editable as its model
choice: a pencil on its dashed bubble opens it in place, the same shape as
editing an already-sent prompt ([014](014-transcript-messages.md)), minus that
editor's warnings about rewinding the agent or the disk — nothing has run yet,
so there is nothing to undo.

`Ctrl+E` opens the last of them without reaching for the pencil, from the
prompt box or from the transcript above it: the prompt just enqueued is where
a typo is noticed, and the box is where the caret already is. It is rebindable
like any other chord ([015](015-settings-shortcuts.md)), and it is left alone
over a file tab, where `Ctrl+E` belongs to the editor. A row the server has
not answered for yet has no id to rewrite, so a press during that round trip
does nothing rather than opening the row above it — the same reason its
pencil is disabled ([024](024-instant-prompt-echo.md)). The prompt bar names
the chord only while something is waiting, since that is when it does
anything.

Leaving that editor, saved or cancelled, puts the caret back in the prompt
box. A keyboard round trip that ends with the focus on nothing is one that has
to be finished with the mouse.

Each queued prompt also records the provider, model and effort selected when
it was enqueued. Its bubble shows the model and offers the same cross-provider
`ModelSelection` control, including fuzzy subscription/provider groups,
favourites and a separate effort picker. Editing one item — its text or its model —
does not change the task's default or other queued items. The PATCH that saves
either kind of edit carries all four fields at once, built from the row
already on screen with just the changed ones overridden, so a text edit keeps
the model choice and a model or effort change keeps the text.

When the runner claims an item, it applies that saved choice immediately before
starting the turn. A provider change clears the provider-owned thread id; a
model or effort change within the current provider keeps it.

A prompt whose turn failed because the provider was away goes back in this same
queue with a clock on it; see [045](045-provider-outage-retry.md).

## Pausing a project queue

The project row has a Pause button immediately left of visibility while tasks
are running. Once paused, it offers Resume while tasks are running or prompts
are waiting. Empty, idle projects show neither action. Task context menus in
the sidebar and project page offer the same project-wide action.

Pause lets every active turn finish but prevents automatic dispatch, including
follow-up prompts in the task that just finished and prompts in other tasks.
New queued prompts and provider retries also wait. Resume keeps the current
task's priority and restarts dispatch once active turns finish. Other projects
keep running.
Explicit Send, Send now and Force send still start work on request.

`PATCH /api/projects/:id` accepts `queue_paused`. The flag is persisted across
restarts and included in project detail and list responses. The runner changes
it under the project's dispatch lock, which also guards the check before
claiming the next queued turn. Project-change events refresh other windows.
