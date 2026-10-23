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
  cancelling the turn (Force send), and answers `joined` with the remaining
  queue. The browser then updates only the queue: reading the transcript back
  would land after rows streamed meanwhile and replace them, the prompt
  included. Enqueue still waits for the turn to end, and the project's queue
  still waits for it too.
- The provider sends `follow_up` as it starts on each prompt, in order. The
  runner stores the prompt then, as a user message of the same turn, so it
  lands after any reply it waited behind. The browser moves the bubble drawn
  on send to that place.
- A prompt that reaches the turn just as it stops taking prompts waits for it
  to end, up to 30 seconds, and starts the next turn.
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
reply instead of waiting idly. Only the turn's first prompt is requeued if the
provider turns out to be away ([045](045-provider-outage-retry.md)).

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
prompts sent and queued meanwhile. Runner tests cover follow-up order, a prompt
missing a closing turn, and Send now joining an open turn. Runner
tests cover prose boundaries, saved state and stream message identity;
store tests cover interrupted tasks on restart. Browser unit tests cover
updates during streaming and duplicate events. Each provider's parser has
unit tests built from lines its CLI actually wrote.
