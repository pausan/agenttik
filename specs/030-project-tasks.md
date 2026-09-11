# Tasks on the project page

A project page is four tabs under its name: **Tasks**, which is every task the
project has; **Jobs**, which is every schedule it has ([below](#the-jobs-tab));
**Prompt**, the standing instructions every task here opens with
([047](047-project-prompt.md)); and **Stats**, its totals and daily activity.

Tasks is one list. Open tasks come first, in the order they were dragged into
([006](006-sidebar-ordering.md)), drawn in the strongest text the theme has.
Archived tasks follow, newest first, drawn grey. The colour is what says which
is which, so the working list and the history read as one place instead of two
stacked sections — the question "where is that task" is asked without knowing
first whether it was archived.

It is drawn a page at a time: 25 rows by default, changeable to 10, 50 or 100
beside the filter. The choice is remembered across launches, because how much
of a list someone wants to see at once is a preference rather than a question
worth asking again on every project.

The page opens with the cursor in the filter, and returning to the Tasks tab
puts it back there. Opening a project is asking which task.

An archived row carries one line the others do not: what the task came to,
drawn under its title in a lighter italic and written when it was archived. A
row whose task was archived before there was a summary, or whose summary could
not be written, simply has one line fewer. See
[051](051-task-outcomes.md).

A project with no tasks at all shows an empty state; a project with no archive
simply has no grey rows.

## The filter

The same fuzzy matcher the folder picker and the Settings panes use
(`fuzzyAny` in `web/src/fuzzy.js`), against the row's **title** alone — a row
with no title matches "Untitled task". Prompts are deliberately left out: one
is up to 600 characters, and a subsequence match against a string that long
matches nearly anything typed. The prompt each row opened with is still one
hover away — [038](038-task-naming.md) describes the same tooltip in the
sidebar.

It narrows both states at once, and the counter beside it reads
`matches/total` over all of them. Matching does not reorder: open rows keep
their position and archived rows stay newest-first, because a best-match row
jumping to the top loses the order each list is kept in for a reason.

Rows are dragged to reorder, and dragging is offered only while the filter is
empty: the drop lands a row where the pointer is *in the whole open list*, and
a hidden neighbour makes that meaningless. The grip greys out and says so
while a filter is typed. A drag inside a page is still exact — the row moves
against the full list by id, not by its position on screen — but a row cannot
be dragged onto another page.

## Paging

Open and archived rows page as one sequence, so a page boundary falls wherever
it falls rather than each state getting a pager of its own. `1–25 of 63` sits
beside the pager under the list, and the counter above still reads
`matches/total` — one says which slice, the other says what the filter cut.

Three rules keep the page honest:

- Typing in the filter, or asking for a different page size, goes back to
  page 1. Both are new questions, and the answer to a new question starts at
  the top.
- A list that shrinks under the page in front pulls it back to the last page
  there is, so archiving the only row on page 3 lands on page 2 rather than on
  an empty page.
- Switching to another project resets the filter and the page and re-focuses
  the filter. One `ProjectView` serves every project — the component is handed
  a new tab rather than mounted again — so what belongs to the page in front is
  put back by hand.

The page size select is only drawn once a project has more tasks than the
smallest size, and the pager only once there is more than one page. A project
with a handful of tasks is still just its tasks.

Paging is a slice of a list already in the browser, so turning a page fetches
nothing. That is bounded by what the project tab loaded: `/api/sessions`
answers at most 200 rows per request, so a project with more than 200 archived
tasks pages through the newest 200 of them. Going past that means paging on
the server — an offset and a total count on the endpoint — which nothing yet
needs.

Archived rows are never draggable. `sessions.position` is the order someone
chose for the work in front of them; a history is ordered by the clock. They
carry the restore icon rather than the archive one, so this is also where a
task comes back: unarchiving turns the row black in place.

## Deleting

Every row, open or archived, also carries a trash icon beside the rename and
archive ones. It asks first: a dialog names the task and warns that its
history goes with it, and only its own **Delete** removes anything — closing
the dialog or its **Cancel** leaves the row untouched.

Confirming removes the task from this list, the sidebar and the tab strip at
once. Unlike archiving there is no way back: `Ctrl+Shift+T` has nothing to
restore, and a tab already open on the task closes with it — see
[007](007-task-closing.md) for what closing unsaved on the way out means.

## The jobs tab

The project's scheduled jobs ([028](028-scheduled-jobs.md)) read like the
tasks beside them: what is still scheduled first, in the order the sidebar was
dragged into, then what has been archived, newest first and grey. The sidebar
carries only the open ones, so this is the only place an archived job can be
seen, and the restore icon on its row is the only way back.

There is no filter and no pager here. A project's tasks run to hundreds; its
jobs are made by hand one at a time, and a handful of rows is a list, not a
search problem.

A row says what its clock is and what is being done with it — the next run,
*paused*, or when it was archived — and carries the same rename the sidebar
row does. An archived row has no Pause or Resume, since archived already means
paused.

## Data

`GET /api/sessions` takes `only_done=true` beside the existing
`include_done`. It is a third list rather than a client-side split of one
`include_done=true` request: the default `limit` of 200 would otherwise cut
rows by `position`, which could drop *open* tasks from a project with a long
history.

An archived-only listing is ordered `last_active_at DESC` even when it is
scoped to a project, where every other project-scoped listing is ordered by
`position`. The timestamp shown on the row is the one it is sorted by.

A project tab loads and reloads totals, daily metrics, open tasks, archived
tasks, and the project's jobs in one `Promise.all`. The reload is the existing
debounced one that already runs at the end of a turn, and a schedule that
changes — archived, renamed, or moved by a fire — runs it too.

The jobs are one request carrying both states, where the tasks take two. The
split exists because `limit` could otherwise cut open tasks out of a project
with a long history; jobs are created by hand and a project has a handful, so
one `include_done=true` listing is the whole list.
