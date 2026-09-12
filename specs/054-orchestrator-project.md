# Orchestrator project

An optional app-wide project coordinates work across other projects using the
same task, schedule, file and provider machinery. `projects.kind` identifies
it independently of its editable name and folder. A partial unique index
allows at most one. Active project lists always put it first, ahead of the
user's ordinary project order.

## Lifecycle and storage

The first enable creates an empty `orchestrator/` folder inside the app's data
directory and a project named Orchestrator with the built-in standing prompt.
Reading settings creates neither. Names, paths and prompts remain editable
through the ordinary project API.

Disabling archives the project, preserving tasks, schedules and options.
Archiving it through the ordinary API also disables it; restoring re-enables
it. Active or queued work prevents archiving. Schedules follow the existing
archived-project rule and stop firing until restoration.

Deleting follows ordinary project deletion: tasks and schedules are removed,
processes are stopped and **no disk files are removed**. Before the cascade,
the store saves the name, folder and prompt in the singleton
`orchestrator_config` table. Re-enabling creates a project with those options
and fresh task history. An intentionally empty prompt stays empty.

The live project is authoritative while present; the singleton holds options
only for recreation. Enabled state is derived from the live row, preventing
ordinary archive, restore or delete calls from leaving settings stale.
Preferences are held in SQLite, shared across windows and retained on restart.
The stable kind and saved instructions are separate from process connection
details; server synchronization is not implemented.

## Instructions and control

`app/internal/orchestrator/default.md` is embedded in the executable. Reset
copies the current build's default into the project prompt, or into saved
options when the project is deleted. Reset does not enable the project or
write files. As with every project prompt, an existing conversation keeps the
copy it started with; changes apply to new conversations.

Every orchestrator turn additionally receives current executable, data
directory, project and task details. These are separate from the saved prompt
and user transcript and are refreshed even on resumed turns. Ordinary tasks
and isolated title/summary requests do not receive them.

The default prompt explains `agenttik --api` ([044](044-command-line.md)) and
the existing project, task and schedule endpoints: inspect live state, read
transcripts, create and start tasks, queue, stop, archive, restore and delete.
These commands reach the running app, so they use the same validation and
runner as the UI. Provider permission modes remain unchanged. No exposed
server configuration is needed.

Project mutations publish `projects_changed`. Task creation, editing and
deletion publish `session_changed`, including the task ID and current row
(absent for a deletion). These reach all windows through the global projects
topic. Turn and schedule activity continues to use existing events.

## Settings API

- `GET /api/orchestrator`: enabled, project_id (0 after deletion), name, path,
  prompt. Before the first enable it returns defaults.
- `PUT /api/orchestrator {"enabled":true|false}`: enable or disable.
- `POST /api/orchestrator/prompt/reset`: restore the current built-in prompt.

Both mutations return the updated settings. A folder collision or a path that
cannot be created fails without enabling or adopting an ordinary project.

## Validation

Go tests cover concurrent enabling, ordering, folder collisions, persistence,
empty/custom/default prompts, archive/delete/re-enable, file retention,
per-turn context, cross-project task control and change events. The command
client tests exercise JSON input, response and error forwarding, and discovery
without database writes. Live model decisions require a signed-in provider
and are not exercised by these tests.
