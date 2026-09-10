# Project archiving

## Outcome

A project can be put away without being deleted. **Archive project** sits
above **Delete project** in the Options pane and takes one click: the project
leaves the sidebar and the Go To list, its tabs close, and its schedules stop
firing. Nothing is destroyed — its tasks, its transcripts, its stats and its
place in the dragged order all wait for it.

**Settings › Projects** is where they wait. Each row is a name, its folder,
how long ago it was archived, and a **Restore** button that puts it straight
back in the sidebar. The rail's filter reaches these rows like any other
setting: typing a project's name or a fragment of its path narrows the list
and the count beside *Projects* says how many matched. With nothing archived
the section says so and points at the Options pane.

Delete is unchanged and still the destructive one: it asks for confirmation
and removes the project's tasks and history from agenttik. The folder on disk
is untouched by either.

Archive is refused while the project is busy — any task running or with a
queued prompt, or a schedule mid-run. The button is disabled and says why.
Archiving would take those rows out of the sidebar, and with them the only
way to reach a turn and stop it; a running task's own archive icon follows the
same rule.

Restoring does not pull the user away from what they were doing: the project
reappears in the sidebar where it was, and the selection only moves to it when
nothing else is selected — restoring the last project of an empty sidebar, for
instance. Archiving the selected project hands the selection to whatever is
left at the top.

## Choices

**A column, not a table.** `projects.archived_at` is 0 while the project is
active and the time it was put away otherwise, matching `sessions.done_at` and
`schedules.done_at`. `ListProjects` filters on it, so every list that already
read that one function — the sidebar, Go To, the SSE subscription, the startup
selection — drops archived projects with no further work. The name is
`archived_at` rather than `done_at` because a project is never *finished*.

No index on it. A database with fifty projects scans the table faster than it
reads a second B-tree, and `projects` is already ordered by
`idx_projects_order`.

**Position is kept, not renumbered.** `ReorderProjects` writes the whole
visible list, so an archived project's `position` is simply not rewritten
while it is away. Restoring puts it back between the same two neighbours
rather than at the top.

**One PATCH, as with the name and the folder.** `PATCH /api/projects/:id`
takes `{archived}` alongside `{name, path}` and applies whichever arrived, so
archiving needs neither of the other two. `GET /api/projects?archived=true`
serves the other list. It skips the per-project session titles and schedules
`ListProjects` attaches: Settings shows a name, a folder and a date, so paying
two queries per row for what nothing draws would be waste.

**The archived list is read when Settings opens.** It cannot change while the
dialog is shut, and `refreshProjects` runs on every turn that starts or ends —
attaching a second query to it would spend a request per turn on a list nobody
is looking at.

**The scheduler joins the flag.** `DueSchedules` already joins `projects` for
the denormalised name and path, so `AND p.archived_at = 0` is free. Excluding
in the query rather than in the runner keeps an idle tick at one indexed
comparison. A project nobody can see must not be starting tasks — and its
schedules resume, unpaused, the moment it is restored.

**Detaching is shared with delete.** `detachProject` deselects the project and
closes its tabs; `removeProject` and `setProjectArchived` both call it, since
nothing in the strip can belong to a project that has left the sidebar either
way.

## Validation

`go build ./...` and `go vet ./...` are clean, and `npx vite build` succeeds.

Browser-checked headlessly against a built web server on a fresh data
directory, with scratch folders `proj-alpha` and `proj-beta`, and no console
or page errors in any run:

| Done in the UI | Result |
|---|---|
| Archive `proj-beta` from its Options pane | leaves the sidebar; `GET /api/projects` no longer lists it, `?archived=true` does, with `archived_at` set |
| open Settings › Projects | one row: name, folder, "just now", Restore |
| filter the rail with `beta` | only *Projects* survives, count 1 |
| Restore | row goes, empty state appears, project is back in the sidebar; selection stays where it was |
| archive the selected project with its page open | tab closes, selection moves to the project left at the top |
| archive the last project | sidebar shows its empty state, centre reads "Pick a task, or a project to start one" |
| restore with nothing selected | the restored project becomes the selected one |

The scheduler rule was checked against the ticker itself: a schedule row due
at `next_run_at = 1` on an archived project stayed unfired over six ticks —
`next_run_at` unmoved, no session, no run row. Restoring the project fired it
inside one tick, advancing `next_run_at` and writing one session and one run.
