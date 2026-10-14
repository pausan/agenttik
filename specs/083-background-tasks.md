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
agent waits on (`is_backgrounded: false`) is not a background task.

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
updates, reloads, finished rows, reused task ids and turn summaries. Runner
tests cover prose boundaries, saved state and stream message identity;
store tests cover interrupted tasks on restart. Browser unit tests cover
updates during streaming and duplicate events. Each provider's parser has
unit tests built from lines its CLI actually wrote.
