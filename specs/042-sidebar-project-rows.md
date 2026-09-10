# A project added lands last, and its letter folds it

## Outcome

How the Projects sidebar draws its list of projects, rather than what is in
them.

**A project added lands at the end.** `projects.position` defaulted to 0 and
the list is ordered by position then name, so every new project sorted above
the ones already dragged into place and then jumped around alphabetically
among the other new ones — a project added twice a day kept taking the top row
and, with it, `Alt+A`. `CreateProject` now takes `MAX(position) + 1` in the
insert itself, the way sessions and schedules already do. Archived projects
count towards the maximum, so restoring one cannot collide with the number of
a project added while it was away.

**A hairline separates one project from the next.** Blocks sit `py-1` apart
with a one-pixel rule between them, nine pixels of space in all: enough to
group the rows under their project, tight enough that eight of them fit a
narrow column without reading as a table. The rule is `border-muted`, not
`border-default` — in dark mode the latter resolves to `neutral-800`, exactly
the sidebar's own background, so it would draw nothing on the theme where the
blocks are hardest to tell apart.

**Task titles hold still when the selection moves.** The number column carries
the `Alt+1` … `Alt+9` shortcut, which only the selected project's first nine
tasks answer to, so drawing the column only there slid every other project's
titles left by a digit's width each time the selection changed. `taskNumber`
returns `0` for every project now — a row in a numbered list that no chord
reaches — and `TaskRow` keeps the twelve-pixel column whether or not it has a
digit to put in it. `null` still drops the column, for the lists that are not
numbered at all: a project's own page and the archive.

**The letter that reaches a project also folds it.** `Alt+A` … `Alt+H` still
address the first eight rows by position. Pressing the letter of the project
already showing now folds its tasks and schedules away, and pressing it again
brings them back, so the chord that reaches a project is also how it is shut —
the chevron is no longer the only way. A different letter only switches, and
leaves every project's fold state where it was.

The fold state moved from the sidebar component to `S.collapsedProjects`,
since the keyboard handler in `App.vue` reaches it now as well as the chevron.
It is per session and not written to `localStorage`: which projects are folded
is where the user is in the list, not a preference. A held letter is consumed
but acts once, like `Ctrl+W`, since repeats would only flap the tasks open and
shut.

## Validation

`go vet ./...`, `npx vite build`.

Browser-checked headlessly, twice, against a fresh database and a fake
`claude` that sleeps so the two tasks stay put. Adding *zeta*, *alpha* and
*mid* in that order through the Add project dialog draws them in that order
with positions 1, 2, 3 — before the change SQLite returned them
alphabetically. Every block after the first computes `border-top-width: 1px`,
at `neutral-200` on the light sidebar and `neutral-700` on the dark one, and
the first computes `0px`. Across two projects of three tasks each, selecting
either one leaves all six titles at the same left offset and every number
column twelve pixels wide, whether it holds a digit or nothing. With
project *one* showing and two tasks under it: `Alt+B` selects *two* and folds
nothing, `Alt+A` comes back without folding, `Alt+A` again hides both task
rows, a third press restores them, `Alt+B` leaves *one* folded as it was, the
chevron still toggles, `Alt+T`/`Alt+P` and `Alt+2` are unaffected, and holding
`Alt+A` for over a second toggles once. No console or page errors.
