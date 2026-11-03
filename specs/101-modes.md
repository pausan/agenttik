# Modes

A mode is a reusable prompt kept apart from any project. It sets how a task
works: a role, rules, domain context, a process, a style of answer, or
anything else worth saying at the start. Projects link to one, a task can pick
another before its first prompt, and Claude Code can launch any mode as a
subagent. Modes extend the project prompt ([047](047-project-prompt.md)); they
do not replace it.

A mode is only words for the agent. It does not change tool access, the run
mode or the permission mode ([082](082-tool-approvals.md)), and Settings says
so, since "mode" means those in Claude Code too.

The name was kept knowing that it reads narrower than what a mode can hold. The
Settings text and placeholders give examples of several kinds, so the name is
not the only explanation of what a mode is for.

## The library

Settings → **Modes** shows one mode at a time, picked from a select by title,
because a library of long prompts stacked in a column is mostly scrolling. The
shown mode lists how many projects link to it.

- **New mode**, beside the section's title, swaps in an empty form with
  Cancel. It is kept off the row of the shown mode, whose select, link count
  and Delete all act on that mode. A mode can also be added from a
  project's Prompt page with **Save as mode…**.
- The title and the text are edited in place and saved on blur. An emptied
  field is put back.
- Titles are unique, ignoring case.
- Delete asks for a second click on the same button. The select then moves to
  the first remaining mode.
- The Settings filter narrows the select.

The library belongs to the profile, as projects do. In the flat view
([100](100-flat-view.md)) `/api/modes` follows the project in front, like the
model picker and Settings.

## Project mode

The Prompt page has a **Mode** select above the project's own text. The linked
mode's whole text is shown read-only between them, scrolling past half the
window's height. **Edit mode** opens Settings on that mode, and **Manage modes**
opens Settings when there is none. What a task opens with is the
mode's text, a blank line, then the project's text; either may be empty.

The project links to the mode rather than copying it, so editing a mode
reaches every project that uses it. As with the project prompt, only
conversations yet to start see the change: each task keeps the copy it opened
with.

**Save as mode…** appears when the project has its own text and no mode. It
asks for a title, creates the mode from that text, links the project to it and
empties the project's own text, so the project's tasks are told the same as
before. A project with a mode does not offer it, since that would drop the link.

## Task mode

The prompt bar of a task that has not started shows a mode select beside the
model: the project's mode (named `<title> (project)`), **No mode** when the
project has one, and every other mode. Once the first prompt is sent or queued
the select is disabled and names the mode the task runs in, by the title the
task copied rather than by its link, which can point elsewhere later; the
server refuses a change after that. A task started in no mode has none to name
and loses the select.

A new task always opens in its project's mode. Unlike the model, effort and
subscription ([025](025-blank-task-reuse.md)), the mode of the newest task is
not carried over: landing back on a blank task resets it too. Opening in a mode
by accident is easier to miss than a model.

When the first prompt is accepted the task copies the resolved text into
`project_prompt`, and the mode's title into `mode_title`. The transcript chip
reads *`<title>` mode added* instead of *Project prompt added* when there was
a mode, and opens that mode in Settings rather than the project's Prompt page:
the task may have run a mode the project does not link to. The mode is the one
the task kept, found by its ID, or by the copied title when the task deferred
to its project. One since deleted or renamed leaves the chip on the project's
prompt, where a task with no mode goes. A task whose mode is deleted before the
first prompt falls back to its project's.

Pinned prompts and schedules ([062](062-pinned-prompts.md),
[028](028-scheduled-jobs.md)) carry a mode the same way. A prompt bar action
copies the task's choice, their views have a Mode select, and every task they
start opens with it.

## Subagents

Every Claude Code turn (except isolated title, summary and conflict requests)
gets `--agents` with one subagent per mode:

- The name is the title in lower case, with runs of other characters turned
  into one hyphen. An empty result becomes `mode`. A name already taken gets
  `-<id>` appended, and again if that name is taken too.
- The description says to use the subagent only when the user asks for the
  mode or the subagent by name. Modes are the user's choice, not the agent's.
- The prompt is the mode's text, which becomes the subagent's system prompt.

So "launch the code-review subagent on this diff" works without writing any
file into the project or `~/.claude`. The flag goes on every turn, resumes
included, because definitions live in the process. Other providers ignore it:
their tasks use modes, but their subagents do not.

The flag is given a path, not the JSON: under `-p` the CLI reads the modes from
a file, written to the system temp directory before the turn starts and removed
when it ends. A command line could not hold a large library — Windows caps the
whole line at 32,767 characters, counted after escaping, and Linux each
argument at 128 KiB — and the file keeps every mode whatever its length. A file
that cannot be written costs the turn its subagents, not the turn.

The orchestrator ([054](054-orchestrator-project.md)) can create a task in a
mode: `POST /api/sessions` takes `mode_id`, and `GET /api/modes` lists them.

## Moving a project

A move ([067](067-local-profiles.md#moving-projects)) carries the modes the
project, its tasks and its schedules use. A destination mode with the same title
is taken as the same way of working and kept as it is. Any other mode is copied
in. Images referenced by those modes are copied like the project prompt's.

## Data and API

`modes(id, title UNIQUE NOCASE, body, created_at)`.
- `projects.mode_id` is a mode or 0 for none.
- `sessions.mode_id` and `schedules.mode_id` are 0 for the project's mode, -1
  for none, or a mode.
- `sessions.mode_title` is the copied title.

Deleting a mode resets every reference to 0 in one transaction.

| Method | Path | Body |
|--------|------|------|
| GET | `/api/modes` | — |
| POST | `/api/modes` | `{title, body}`, both required |
| PATCH | `/api/modes/:id` | `{title?, body?}` |
| DELETE | `/api/modes/:id` | — |

`mode_id` is accepted by `POST /api/sessions`, `PATCH /api/sessions/:id` (before
the first prompt only), `PATCH /api/projects/:id`, `POST /api/schedules` and
`PATCH /api/schedules/:id`. Library changes publish `modes_changed` on the
projects topic, and a deletion also publishes `projects_changed`. The client
reads the library after the first paint, outside the launch budget
([052](052-launch-budget.md)).

## Verification

- Store tests cover unique titles, unlinking on delete and moves into a library
  that already has a same-titled mode.
- Runner tests cover resolution (project's, task's, none, edits after start)
  and subagent names.
- Server tests cover validation and the first-prompt cutoff.
- The browser test covers Save as mode, picking a mode, reset on reuse, the
  chip and deletion from Settings.
- `--agents` was checked by hand against Claude Code 2.1.283 in print mode.
