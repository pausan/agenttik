# Tasks on the project page

## Outcome

A project page is two tabs under its name: **Tasks**, which is every task the
project has, and **Stats**, which is its totals and daily activity.

Tasks is one list. Open tasks come first, in the order they were dragged into
([006](006-sidebar-ordering.md)), drawn in the strongest text the theme has.
Archived tasks follow, newest first, drawn grey. The colour is what says which
is which, so the working list and the history read as one place instead of two
stacked sections — the question "where is that task" is asked without knowing
first whether it was archived.

The page opens with the cursor in the filter, and returning to the Tasks tab
puts it back there. Opening a project is asking which task.

A project with no tasks at all shows an empty state; a project with no archive
simply has no grey rows.

## The filter

The same fuzzy matcher the folder picker and the Settings panes use
(`fuzzyAny` in `web/src/fuzzy.js`), against the row's **title** alone — a row
with no title matches "Untitled task". Prompts are deliberately left out: one
is up to 600 characters, and a subsequence match against a string that long
matches nearly anything typed. The prompt each row opened with is still one
hover away ([014](014-transcript-messages.md) draws the same tooltip in the
sidebar).

It narrows both states at once, and the counter beside it reads
`matches/total` over all of them. Matching does not reorder: open rows keep
their position and archived rows stay newest-first, because a best-match row
jumping to the top loses the order each list is kept in for a reason.

Rows are dragged to reorder, and dragging is offered only while the filter is
empty: the drop lands a row where the pointer is *in the whole open list*, and
a hidden neighbour makes that meaningless. The grip greys out and says so
while a filter is typed.

Archived rows are never draggable. `sessions.position` is the order someone
chose for the work in front of them; a history is ordered by the clock. They
carry the restore icon rather than the archive one, so this is also where a
conversation comes back: unarchiving turns the row black in place.

## Data

`GET /api/sessions` takes `only_done=true` beside the existing
`include_done`. It is a third list rather than a client-side split of one
`include_done=true` request: the default `limit` of 200 would otherwise cut
rows by `position`, which could drop *open* tasks from a project with a long
history.

An archived-only listing is ordered `last_active_at DESC` even when it is
scoped to a project, where every other project-scoped listing is ordered by
`position`. The timestamp shown on the row is the one it is sorted by.

A project tab loads and reloads totals, daily metrics, open tasks, and
archived tasks in one `Promise.all`. The reload is the existing debounced one
that already runs at the end of a turn.

## Validation

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (33) pass.

Headless against the running app, one project with two open and three
archived tasks:

- The Tasks tab listed all five, counter `5/5`, with the two open titles at
  `oklch(0.21 …)` — the theme's `text-highlighted` — and the three archived at
  `oklch(0.552 …)`, its `text-muted`.
- The page opened with focus on `input[type=search]`, placeholder
  `Filter tasks`, so typing filtered without clicking first.
- `arch` narrowed to the three archived rows and the counter to `3/5`; `one`
  to the single open row and `1/5`; `zzzz` drew "No task matches that."
- No console or page errors throughout.
