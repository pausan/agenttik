# Project workspaces, tab groups, and file diffs

## Outcome

The tab strip is one project's workspace rather than everything open at once.
`S.activeProjectID` selects it, `S.strip` is `S.tabs` narrowed to that project,
and every other tab stays open, live and streaming while off screen. Switching
project restores the tab it was last left on (`S.lastTab`), and a project with
nothing open opens its own page rather than leaving the centre blank. Closing
the last tab in a workspace falls back to the page the same way — unless the
page is what was closed, which would otherwise be impossible to get rid of.

Inside a project the tabs are grouped by kind — page, conversations, files —
and coloured sky, green and amber. `insertTab` puts a new tab at the end of its
own group, so a new session always lands after the last session and before the
files. `moveTab` refuses to move a tab into another group, so a drag can
rearrange conversations among themselves without breaking the order.

Numbers are positions, not identities: `Alt+1 … Alt+9` index `S.strip`, so a
drag changes what a chord reaches and each project counts from 1 again.
`Alt+A … Alt+H` index `S.projects`, so dragging a project row changes its
letter too. Both are read from the physical key code.

The sidebar is the canonical order for a project's sessions. The project page
and the session-tab subset are kept in that order whenever either is dragged,
then one project-session order request is sent at drop. Selected-project rows
show their matching one-based number. Session tab labels are shortened to 20
characters with `...`; project and file tabs keep their complete names and
scroll horizontally when needed.

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
project's tabs interleaved, and the invariant is per project: each project's
own tabs are in group order relative to each other.

**A file tab carries its project id.** It used to be derived from the view it
was opened from, which is nothing once that view closes — and files can now be
opened from the Tree with no tab open at all, since the right panel and Tree
fall back to the selected project.

**The strip is hand-written rather than `UTabs`.** Per-tab drag handlers, a
colour per kind and a live number were all fighting the component's slots. It
keeps `role="tablist"`/`role="tab"` and gains an `aria-label` of the untruncated
label, so a tab is addressable by its full name.

## Validation

`npm run build` passed. No tests were run.
