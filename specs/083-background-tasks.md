# Background tasks

An agent can start a shell or a subagent and move on while it runs: a dev
server, a long test run, an exploring subagent. Each task has a visible row in
the conversation where the provider first reports it, between the prose and
tools that surround that event. This is approximate start order: a provider
can report a task after it started. A task separates consecutive tool groups
so collapsing tools does not hide or move it. Each row shows:

- a terminal icon for a shell, a bot icon for a subagent;
- the command or description;
- the start time, and how long it has run, ticking with the progress line;
- once it ends, its status (`completed`, `failed`, `stopped`) and summary,
  e.g. `failed · exit code 2`, in the error colour when it failed;
- an **Output** link when the provider names an output file, which opens it
  in a file tab.

Status updates change the original row in place. Rows stay after the turn
ends and after reloads. Tasks belong to the turn and die with its process;
any still running when it exits become `stopped · turn ended`. A restart
marks tasks left running by an interrupted turn `stopped · turn interrupted`.

## Talking while tasks run

A reply that ends with background tasks still running leaves the turn open
(Claude Code only; see [003](003-providers.md)). The turn then takes prompts:

- Its done event carries an `agent.FollowUp`. The runner keeps it until the
  turn ends, and `GET /api/sessions/:id` reports `accepts_prompts`; so does
  the stream on that done event.
- Send hands the prompt to the turn instead of refusing it, and answers with
  the running turn. Send now on a queued prompt does the same instead of
  cancelling the turn (Force send), and answers `joined` with the stored
  prompt and the remaining queue. Its queue row stays until the provider has
  the prompt, then becomes the stored prompt in one transaction, so it is never
  in both. The row is looked for under the turn's lock: of two Send nows for
  one row, the second gets 404. The browser then updates only the queue: reading the
  transcript back would land after rows streamed meanwhile and replace them,
  the prompt included. Enqueue still waits for the turn to end, and the
  project's queue still waits for it too.
- The turn's process runs on the choice it started on: provider, subscription,
  model and effort. A prompt joins it only on the same one. Send compares the
  task's current choice, which the picker can change while the turn runs;
  Send now compares the queued prompt's saved choice. On another, the request
  is refused with 409, carrying `running` and `wanted`, and nothing is
  consumed. `on_mismatch: "join"` sends it on the turn's choice anyway. Send
  now also takes `"stop"`, which stops the turn, background tasks included,
  and runs the prompt on its own choice, as Force send does.
- `GET /api/sessions/:id` reports `running_choice`, and the browser takes it
  from the started event's session, so it asks before sending: the dialog
  shows both choices and offers "Send with ‹model›" or Cancel, with how to
  run the prompt on its own model: enqueue it, or Force send it from the
  queue. A queued prompt's Send now also offers "Force send with ‹model›",
  and its row reads "Send now…" with a hint saying why beforehand. The
  buttons name the model and effort, and the subscription and provider too
  when those are what differ, so the two never read the same. A 409 the
  browser did not see coming asks the same. If the turn ends while the
  dialog is open, the answer has no turn to join or stop: Send starts a turn
  on the task's choice, and Send now on the prompt's saved one, which is the
  model each was meant for.
- An accepted prompt is stored before the provider is handed it, as a user
  message without a turn: if the agent is replying, the CLI holds it until
  that reply ends, and only the store would otherwise remember it.
  `GET /api/sessions/:id` lists such prompts in `pending_follow_ups`.
- The provider sends `follow_up` as it starts on each prompt, in order. The
  prompt then joins the turn, below the reply it waited behind: when rows were
  stored after it, it moves under a new id, and the event carries the old one
  in `replaces`. The browser swaps it, or the bubble drawn on send, in place.
- Ids are the transcript's order, so a prompt without a turn changes id
  whenever it moves below rows stored after it: as a turn takes it, as it is
  sent again unprocessed, or as a turn ends without taking it. The first two
  say so with `replaces`, on `follow_up` and `started`; the last is read back
  with the transcript at the end of the turn. An id held for such a prompt is
  only good until then.
- A turn that ends before the agent starts on an accepted prompt — Stop,
  quitting the app, the CLI dying — moves it to the end of the transcript,
  still without a turn. After a provider outage that agenttik retries by
  itself, it goes back to the queue behind the retry instead.
- A prompt of yours without a turn says so above its bubble: "Waiting for the
  agent" while the running turn holds it, "Not processed" once nothing will
  take it. That includes queued prompts Stop moved into the transcript
  ([021](021-stop-active-tasks.md)) and prompts left by a crash, but not the
  copies a task opened from a thread starts with ([092](092-threads.md)):
  they ran in the thread, and are older than the task.
- A "Not processed" prompt has Send and Enqueue buttons beside its label. They
  do what the prompt bar does, with the prompt as it is; editing it would cut
  the transcript below it. `POST /api/sessions/:id/messages/:message/send`
  hands it to the running turn when that turn takes prompts, answering
  `joined` with the prompt as stored and held like any other, or starts a turn
  with it. While the agent is busy with something else it is refused, as Send
  is. The message itself moves to the end of the transcript, where a prompt
  sent now goes; a turn it starts carries it on its started event, with
  `message` and `replaces`. `POST /api/sessions/:id/messages/:message/queue`
  moves it out of the transcript to the end of the task's queue, with the
  task's current model. Neither reorders anything else: Send starts beside the
  project's other tasks, as Send always does, and Enqueue waits for the
  project's queue. Both refuse, with 409, a prompt that ran, one the running
  turn holds, or one that is not yours.
- A prompt that reaches a turn as it ends — it has just stopped taking
  prompts, or its last reply is out — starts the next turn: Send waits up to
  30 seconds for the turn to end, and Send now claims the turn's release, as
  Force send does, without cutting it short. This holds for any turn, with
  background tasks or not. A turn still replying is busy, and refuses at once.
- The prompt bar's Send stays enabled, and the progress line says how many
  tasks are running, and whether the agent is replying or waiting. A
  background subagent keeps calling tools while the agent waits; those calls
  are marked `subagent` and do not count as the agent replying. Stop still
  kills the CLI and its tasks.
- Task rows, in the sidebar and on the project page's Tasks tab, show a
  layers icon and the number of tasks still running; the tooltip adds "you can
  keep writing" while the turn takes prompts. The project listing and
  `GET /api/sessions` carry `background_tasks` and `accepts_prompts` for each
  running task, filled from the runner. When either changes, the runner
  publishes `session_background` on the project topic. The browser then reads
  the sidebar's listing again, and the task list of any open tab of that
  project, once per 150 ms.

The task prompt tells the agent the user can write meanwhile, so it ends its
reply instead of waiting idly. If the provider turns out to be away, the input
the failure interrupted is requeued: the turn's prompt, or the follow-up being
answered, never one already answered ([045](045-provider-outage-retry.md)).

## Event

Providers send `agent.EventBackground` with an `agent.BackgroundTask`: id,
kind (`shell` or `agent`), description, status (`running`, `completed`,
`failed`, `stopped`), start and end times in Unix milliseconds, summary and
output file. Each event carries the task's whole state, and the latest one
wins.

The runner stores one `background` message per task per turn. Its content is
the JSON task state. Before inserting its first row, it flushes assistant
prose; updates change only content, preserving the message id and creation
time. The stream carries this message alongside the background event. The
browser inserts or replaces it by message id without interrupting current
prose on updates. Task ids reused in later turns get separate messages.

The runner also keeps live state by task id until the turn ends.
`GET /api/sessions/:id` returns both stored `messages` and the live
`background` list. The transcript uses messages, so live and finished tasks
share the same ordering after a reload. Editing an earlier prompt removes
later task rows with the rest of that transcript.

## Providers

**Claude Code** names its tasks. `background_tasks_changed` lists the running
ones. `task_started` (with `is_backgrounded`), `task_updated` (`patch.status`,
`patch.end_time`) and `task_notification` (`status`, `summary`,
`output_file`) follow one task. A task is reported when it first appears. It
is marked completed when it leaves the list, and the notification then sets
how it really ended. The CLI's `killed` reads as `stopped`. A subagent the
agent waits on (`is_backgrounded: false`) is not a background task. A reply
keeps the turn open while tasks remain active, using both list and individual
status updates. Each task prompt tells the agent to stop disposable helpers
once its requested work is complete, while waiting for work it still needs
and respecting processes the user asked to keep running. There is no automatic
timeout or cancellation based on the reply's wording.

**Codex** has no background flag. Unified exec runs a command in a PTY. When
the command outlives its yield time, the agent carries on and the
`commandExecution` item stays `inProgress` until the process exits. A command
becomes a background task when a later root-thread item starts after the
command has run for at least a second, or when the agent writes to its
terminal (`item/commandExecution/terminalInteraction`). Calls made in
parallel start within milliseconds of each other and so stay foreground. The
item's completion ends the task: `failed` on a non-zero exit code, with
`exit code N` as the summary.

**Copilot** starts background work with ordinary calls: `bash` (or
`powershell`) with `mode: "async"`, and `task` with `mode: "background"`. The
task starts when the call succeeds, identified by the `shellId` or `agent_id`
in its result. A `system.notification` ends it: `shell_completed` (with exit
code), `shell_detached_completed`, `agent_completed` (with status) or
`agent_idle`. A successful `stop_bash` marks the shell stopped.

**OpenCode** reports none. Its `task` tool has a `background` option, but
1.18 does not offer it to the model, and `opencode run` exits after its reply
anyway.

## Testing

The fake provider's `@background start <id> <description>` and
`@background end <id> <status> <summary>` directives drive
`e2e/tests/background-tasks.spec.js`, covering placement between tools,
updates, reloads, finished rows, reused task ids and turn summaries. Its
`@open` directive keeps the turn open while a task runs, for the test of
prompts sent and queued meanwhile, for unprocessed prompts sent and queued
again, and for the dialog of a prompt meant for another model. It holds up to
eight prompts at a time and refuses the next, as a turn that is ending does.
Runner tests
cover follow-up order, a prompt missing a closing turn, Send now joining an
open turn, sending or queueing an unprocessed prompt into each state of a
task, and joining only on the turn's choice unless told what to do. Runner
tests cover prose boundaries, saved state and stream message identity;
store tests cover interrupted tasks on restart. Browser unit tests cover
updates during streaming and duplicate events. Each provider's parser has
unit tests built from lines its CLI actually wrote.
