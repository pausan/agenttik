# Background tasks

An agent can start a shell or a subagent and move on while it runs: a dev
server, a long test run, an exploring subagent. While the turn runs, these are
listed under the progress line ([010](010-working-indicator.md)). The list is
closed by default and shows one button: "2 background tasks running", or
"2 background tasks finished" once none is left running. Opening it shows one
row per task:

- a terminal icon for a shell, a bot icon for a subagent;
- the command or description;
- the start time, and how long it has run, ticking with the progress line;
- once it ends, its status (`completed`, `failed`, `stopped`) and summary,
  e.g. `failed · exit code 2`, in the error colour when it failed;
- an **Output** link when the provider names an output file, which opens it
  in a file tab.

Tasks belong to the turn. They die with its process, and the list goes when
the turn ends.

## Event

Providers send `agent.EventBackground` with an `agent.BackgroundTask`: id,
kind (`shell` or `agent`), description, status (`running`, `completed`,
`failed`, `stopped`), start and end times in Unix milliseconds, summary and
output file. Each event carries the task's whole state, and the latest one
wins.

The runner keeps each running session's tasks in memory, by task id, and
drops them when the turn ends. `GET /api/sessions/:id` returns them as
`background`, so a window that reloads still sees them. The browser updates
the open tab's `detail.background` from the stream and empties it on `done`.

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
`e2e/tests/background-tasks.spec.js`. Each provider's parser has unit tests
built from lines its CLI actually wrote.
