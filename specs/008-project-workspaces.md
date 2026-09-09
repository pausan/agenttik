# Project workspaces, tab groups, and file diffs

## Outcome

The tab strip is one project's workspace rather than everything open at once.
`S.activeProjectID` selects it, `S.strip` is `S.tabs` narrowed to that project,
and every other tab stays open, live and streaming while off screen. Switching
project restores the tab it was last left on (`S.lastTab`), and a project with
nothing open opens its own page rather than leaving the centre blank. Closing
the last tab in a workspace falls back to the page the same way — unless the
page is what was closed, which would otherwise be impossible to get rid of.

Inside a project the tabs are coloured by kind — page, conversations, files —
in sky, green and amber. `insertTab` puts every new tab at the end of its
project's strip, to the right of the tabs already there. `moveTab` refuses to
move a tab onto one of a different kind, so a drag can rearrange conversations
among themselves.

Numbers are positions, not identities: `Alt+1 … Alt+9` index `S.strip`, so a
drag changes what a chord reaches and each project counts from 1 again.
`Alt+A … Alt+H` index `S.projects`, so dragging a project row changes its
letter too. Both are read from the physical key code.

The sidebar is the canonical order for a project's sessions. New sessions get
the next position, so their row is last. The project page and the session-tab
subset are kept in that order whenever either is dragged, then one
project-session order request is sent at drop. Session tab labels are shortened
to 20 characters with `...`; project and file tabs keep their complete names and
scroll horizontally when needed.

A sidebar row of the selected project carries the number of its tab rather than
its place in the list, so the digit there is the digit that reaches it. Rows
whose conversation is not open, or that sit past the ninth tab, keep the blank
column; rows of every other project have no column at all.

`startSession` refuses to open a second blank conversation: while the one in
front has no messages, no queued prompts and an empty draft, the chord, the
strip's `+` and both New session buttons select it and focus the prompt
instead.

A project row shows its letter dimmed in front of the name, is highlighted when
selected, and pulses a dot on the right while any of its sessions is mid-turn —
`recent_sessions` already carries each session's status, so this costs nothing
extra. The chevron became its own button: the row itself selects the project,
and selecting the already-selected project opens its page, which is the way
back to Options and Delete once conversations are open on top.

A file tab carries `File | Diff`, and the choice is remembered in
`localStorage` and applied to the next file opened. `GET /api/projects/:id/diff`
serves the diff: `git diff HEAD` for a tracked file, `--cached` where there is
no `HEAD` yet, and `--no-index` against `/dev/null` for an untracked one so it
reads as new rather than as unchanged. Only the half on screen is fetched.

An unsent prompt moved from the prompt bar to the session tab. One bar serves
every conversation, so a ref inside it followed the user between tabs; a
`draft` on the tab does not. It is saved with the open tabs, which is why
`saveOpenTabs` is now debounced.

## Choices

**Tabs stayed one flat array.** Grouping them per project in the store would
have touched closing, restoring, subscriptions and streaming; a filter plus an
insertion rule touches only insertion and the strip. The array holds every
project's tabs interleaved, and the invariant is that a new tab follows every
tab already open in its project.

**The sidebar number is the tab number.** It used to be the row's own place in
the list, which read as an `Alt` chord and was not one: the strip counts the
project page and open files too, and numbered rows for conversations that were
not open at all. Positions in the two lists cannot be made to agree — the strip
is a subset with two other kinds of tab in it — so a row shows the strip's
number or nothing.

**A blank conversation is reused rather than counted.** Refusing on the tab in
front, rather than searching the project for any empty session, keeps the rule
to what is visible: the answer to "why did nothing happen" is on screen. Draft
text is what makes a conversation worth keeping — messages and the queue are
already checked, and a session with neither is indistinguishable from the one
the button would create.

**A file tab carries its project id.** It used to be derived from the view it
was opened from, which is nothing once that view closes — and files can now be
opened from the Tree with no tab open at all, since the right panel and Tree
fall back to the selected project.

**The strip is hand-written rather than `UTabs`.** Per-tab drag handlers, a
colour per kind and a live number were all fighting the component's slots. It
keeps `role="tablist"`/`role="tab"` and gains an `aria-label` of the untruncated
label, so a tab is addressable by its full name.

## Validation

`git diff --check` passed. Tests and builds were not run, per the current
prototype policy.

The numbering and the blank-session rule were browser-checked on a fresh
database with three named conversations. With only the project page open every
sidebar row showed a blank column; opening two of them numbered those rows 2
and 3, matching the strip, and `Alt+1`/`Alt+2`/`Alt+3` selected the page, the
first and the second. `Ctrl+N` and `Ctrl+T` on a blank conversation left the
session count at 3 and focused the textarea; typing one letter and pressing
`Ctrl+N` made the fourth, and pressing it again on that blank one changed
nothing. The page logged no errors.
