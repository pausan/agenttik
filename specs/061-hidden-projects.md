# Temporarily hiding projects

The Projects sidebar can temporarily hide a project without archiving or
deleting it. The project row's **eye-off** button sits at the far right,
opposite the project title. Hiding removes the row from the sidebar and from
project navigation, but leaves its tasks, history, schedules and folder in
place. A task that is already running continues to run.

The sidebar footer contains a **Hidden projects** button. Opening it displays a
small command-palette menu with only the hidden project names. Its search field
is fuzzy autocomplete: a subsequence such as `wb` matches `web`, with better
matches first. Selecting a name restores the project and keeps the menu open
so more projects can be restored. Clicking the eye-off button again closes it.
The **Restore all** button restores every hidden project, regardless of the
search filter, and keeps the menu open. It is disabled while loading or
restoring projects and when no hidden projects remain. Restores run sequentially
to preserve the existing saved-row insertion behavior.

The **Swap** button beside **Restore all** shows every hidden project and hides
all currently visible projects, regardless of the search filter. It captures
both lists before changing them and keeps the menu open. It also works when
one list is empty, and is disabled while loading or changing visibility, or
when both lists are empty. Changes run sequentially, hiding visible projects
from bottom to top before restoring the previously hidden projects.

## Ordering

When a project is hidden, its visible sidebar row is saved in
`projects.hidden_position`. Reordering projects that remain visible writes only
their ordinary `projects.position` values, so it cannot overwrite the hidden
project's target row. Restoring inserts the project at the saved row, clamped
between the first and last current rows, and rewrites visible positions densely.
The orchestrator is always clamped to the first row.

## API and storage

The `projects` table has `hidden_at` and `hidden_position`. `hidden_at = 0`
means visible; a non-zero value means temporarily hidden. Archiving clears the
hidden state, so archived projects remain in the archived-projects workflow.

`PATCH /api/projects/:id` accepts `{hidden: true}` or `{hidden: false}`.
`GET /api/projects` continues to return visible projects. The hidden menu reads
`GET /api/projects?hidden=true`, whose rows contain only `id` and `name`.

The hidden list is loaded when its menu opens rather than delaying the initial
sidebar paint. Once loaded, project-change events refresh it for the window.
