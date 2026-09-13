# Pinned prompts

The prompt action menu offers **Pinned task**. It saves the current prompt,
project, provider, subscription, model, effort and permission without starting
a task or making a provider request. The first prompt line supplies its title.

Pinned prompts stay above schedules and tasks in the project sidebar. They
persist across restarts and are excluded from sidebar navigation shortcuts.
The row's **Play** button starts one new ordinary task with the saved settings.
The pinned prompt stays in place. Spawned tasks follow normal task behavior,
including remaining visible after completion.

Clicking the row opens its editor. Prompt edits remain local until **Save
changes**; **Discard** restores the saved text. **Play** uses the saved text.
**Customize and run…** opens a separate copy to edit and send once. Canceling
that dialog creates nothing; sending never changes the saved prompt. Each
click creates a separate task, even while earlier tasks are running.

Pinned prompts can be renamed, archived, restored and deleted using the saved
prompt controls. Deleting a pin leaves its spawned tasks intact.

## Data and API

Pinned prompts reuse `schedules` with `every = 'pinned'`, zero recurrence
fields, zero remaining count and `next_run_at = 0`. The due query explicitly
excludes this mode, regardless of stored counter or pause values. Recurrence,
pause and counter updates are rejected for pins. No schema migration is needed.

`POST /api/schedules/:id/run` creates and returns one ordinary session for a
pin, optionally taking a `prompt` override. Empty overrides and archived pins
are rejected. These tasks have no `schedule_id` or `schedule_runs` entry.

## Prompt actions

Send, Enqueue, Schedule, Pinned task and the action-menu trigger remain disabled
until the text contains a non-whitespace character. Clearing the text disables
them again. Image attachments alone do not enable them. Send and enqueue
keyboard bindings enforce the same check; upload and running guards still
apply. Model selection and Stop remain usable without a draft.

## Verification

Store tests cover ordering, time filtering and exclusion from automatic runs.
Browser tests cover empty controls, creation without running, save/discard,
reload persistence, Play and one-off customization.
