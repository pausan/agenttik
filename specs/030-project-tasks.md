# Tasks on the project page

A project has five panes: **Tasks**, **Archived**, **Jobs**, **Prompt** and
**Stats**. Tasks opens first, with the cursor in its filter. Only project
metadata and open tasks are needed to open or restore a project tab.

## Open tasks

Tasks shows open tasks in their dragged order. It pages locally, with 25 rows
by default and a remembered choice of 10, 25, 50 or 100. The counter shows
matches/total. Filtering or changing page size resets page 1; removing the
last row on a page moves back to the last available page.

Fuzzy Search matches titles; Smart Search ([057](057-smart-search.md)) ranks
open tasks using titles, opening prompts and outcomes. Both offer All times,
Last 24h, Last week and Last month, based on last activity. A task's opening
prompt remains available on hover. Search signatures are only constructed
when Smart Search is enabled.

Dragging is available with an empty text filter and All times selected.
Reordering uses IDs against the whole open list. It updates immediately and
persists on drop; failure reloads the saved order.

## Archived tasks

Archived is an explicit history request. It fetches one page from the backend,
ordered by last activity descending with ID descending as a stable tiebreaker.
The request asks for page size plus one row to enable Next; Previous and Next
replace that page rather than accumulating history. Leaving Archived releases
its rows. Opening other projects or receiving background task events does not
load their archives.

The archive has a server-side substring search for titles (the sessions API
also matches project name and path), debounced by 250 ms, and the standard
activity time windows. These filters run before pagination and reach the
whole archive. This search does not use the open list's fuzzy/semantic mode.
A changed query, time window or page size resets to page 1. Stale answers are
ignored after navigation or unmount. Errors show Retry.

Archived rows show their outcome ([051](051-task-outcomes.md)), can be opened
without restoring, and offer rename, unread, restore and delete actions.
Restoring moves the task to Tasks. Deleting from either pane asks for
confirmation and removes the transcript permanently ([007](007-task-closing.md)).

## Secondary panes and refreshes

Jobs loads the project's schedules, including archived ones, when opened.
Open jobs come first in sidebar order, then archived jobs by archive date.
Jobs have pause/resume, rename and restore controls, without filtering or
pagination. Prompt edits the standing project instructions ([047](047-project-prompt.md)).
Stats loads totals and daily activity only when opened.

Task and schedule events debounce a refresh of open task lists and increment
a project revision. Only the currently mounted history, Jobs or Stats pane
requests its secondary data for that revision. Late responses cannot update
a different pane or project.

## API

`GET /api/sessions` accepts `include_done=false` for open tasks and
`only_done=true` for history. `limit` and `offset` page the filtered list;
a positive offset requires a positive limit. The default limit is 200;
`limit=0` remains available for callers needing all open tasks. The UI's
background session list excludes archived tasks.

`newest=true&limit=1` selects the newest created task for new-task model
defaults, including archived tasks, without transferring the entire history.
