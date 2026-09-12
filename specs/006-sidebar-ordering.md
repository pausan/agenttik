# Sidebar ordering and the project tree

The left strip is titled **Workspace** over `Projects | Tree`. With two or
more managed repositories, a picker under the title switches the selected
project by the same work-first rule as its `Alt` letter; with one repository
it is omitted. The project rows remain the canonical list and drag order.

Tree always uses the project that owns the active tab — a task, a project page,
or a file opened from either — and falls back to the selected project when
nothing is open in it. With no project at all it asks the user to pick one.

The right strip is `Changed | Commits` for a task and `Options | Commits`
for a project. Task Stats opens from the pie-chart button after model effort in
the centre header; see [004](004-ui.md#tabs).

## Dragging

Projects, and every open task under each project, are reordered with native
browser drag and drop. The list moves below the pointer immediately and one
request is sent on drop; if the server refuses it, the row goes back where it
was.

Project order lives in `projects.position` and task order in
`sessions.position`, which serves the sidebar and the project page alike.
Positions are written `1..n`, and a row created afterwards takes the next
number, so new work lands at the end of an order someone arranged rather than
on top of it — see [042](042-sidebar-project-rows.md) for projects.

`GET /api/projects/:id` carries every open task row, not a subset, so a sidebar
drop always sends the complete order.

## Archiving

A task row's archive icon takes it out of the project's sidebar list. Nothing
is deleted: it moves to the grey half of the project page's Tasks tab, where
the restore icon brings it back — see [030](030-project-tasks.md). A task that
is running or holding queued prompts shows **Stop** in place of the archive
icon; see [021](021-stop-active-tasks.md).
