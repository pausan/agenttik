# Archived sessions on the project page

## Outcome

A project page keeps open sessions on top, in the order they were dragged into
([006](006-sidebar-ordering.md)). Below that working list, **Sessions** and
**Stats** tabs keep the project's history and aggregate activity together.

Sessions contains everything the project has archived, newest at the top and
oldest at the bottom, behind its filter. It is project-scoped rather than a
time-windowed list mixed with other projects. A project with no archived
sessions shows an empty state instead of omitting the tab.

## The filter

The same fuzzy matcher the folder picker and the Settings panes use
(`fuzzyAny` in `web/src/fuzzy.js`), against the row's **title** alone — a row
with no title matches "Untitled session". Prompts are deliberately left out:
one is up to 600 characters, and a subsequence match against a string that
long matches nearly anything typed. The prompt each row opened with is still
one hover away ([014](014-transcript-messages.md) draws the same tooltip in
the sidebar).

Matching does not reorder. The list stays newest-first while it narrows,
because the question being asked of a history is *when*, and a best-match
row jumping to the top loses that. A counter beside the bar reads
`matches/total`.

Rows carry the restore icon rather than the archive one, so this is also
where a conversation comes back: unarchiving moves it out of this list and
into the open one above, both of which the same reload re-reads.

Archived rows are not draggable. `sessions.position` is the order someone
chose for the work in front of them; a history is ordered by the clock.

## Data

`GET /api/sessions` takes `only_done=true` beside the existing
`include_done`. It is a third list rather than a client-side split of one
`include_done=true` request: the default `limit` of 200 would otherwise cut
rows by `position`, which could drop *open* sessions from a project with a
long history.

An archived-only listing is ordered `last_active_at DESC` even when it is
scoped to a project, where every other project-scoped listing is ordered by
`position`. The timestamp shown on the row is the one it is sorted by.

A project tab loads and reloads totals, daily metrics, open sessions, and
archived sessions in one `Promise.all`. The reload is the existing debounced
one that already runs at the end of a turn.

## Validation

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (33) pass.
`go test ./...` fails only `TestDoneReachesProjectTopic` and
`TestReorderSessionsDrivesProjectOrder`, both confirmed failing at `HEAD`
before this change in a clean worktree.

Against the running app with six archived and two open sessions in one
project:

- The archived list came back newest-first, the reverse of the order it was
  created in, with the open list above it untouched.
- `arch` narrowed six rows to `Rename the archive icon`, and the counter to
  `1/6`.
- `mgrt` — no substring of anything — found `Migrate the session table`, so
  the match is fuzzy rather than a `LIKE`.
- `zzzz` drew "No archived session matches that." rather than an empty gap.
- Restoring the top archived row moved it to the head of the open list and
  left five behind.

No console or page errors throughout.
