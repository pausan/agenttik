# Project insertion and folding

How the Projects sidebar draws its list of projects, rather than what is in
them.

**A project added lands at the configured end.** General defaults to top.
`CreateProject` chooses `MIN(position) - 1` for top or `MAX(position) + 1`
for bottom in the insert itself. Archived projects count toward both bounds.
The orchestrator remains pinned first. See [017](017-general-settings.md).

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

**A right click on the row opens the project on the desktop.** One item, and a
menu per row rather than the Tree's one menu aimed by the click: a sidebar
holds a handful of projects, and each row already knows which project it is.
The menu wraps the row itself, not the block around it: a right click on the
jobs and tasks below is not aimed at the project — see
[048](048-open-in-system-browser.md).

**The letter that reaches a project also folds it.** `Alt+A` … `Alt+H` still
address the first eight rows by position. Pressing the letter of the project
already showing now folds its tasks and schedules away, and pressing it again
brings them back, so the chord that reaches a project is also how it is shut —
the chevron is no longer the only way. A different letter only switches — onto
that project's latest task, which is what the chord is for; the row itself is
the way to the project page (see [008](008-project-workspaces.md)) — and
leaves every project's fold state where it was, unless the General setting
asks for one project unfolded at a time, which unfolds the one selected and
folds the rest (see [017](017-general-settings.md)). Either way the fold state
is one fact per project, and the chevron and the letter still write it.

The fold state moved from the sidebar component to `S.collapsedProjects`,
since the keyboard handler in `App.vue` reaches it now as well as the chevron.
It lasts as long as the page does and is not written to `localStorage`: which
projects are folded
is where the user is in the list, not a preference. A held letter is consumed
but acts once, like `Ctrl+W`, since repeats would only flap the tasks open and
shut.
