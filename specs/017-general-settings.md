# General settings: which key submits, and what a selection folds

Settings grew a fourth section, **General**, and it is where the dialog now
opens: the sidebar's Settings button and the launcher's Settings entry both
land there, the keyboard button still lands on Shortcuts.

It holds two questions: does the plain Enter send the prompt or queue it, and
does selecting a project fold the other projects away?

## Enter sends, or Enter enqueues

Picking the other way round swaps the pair: Enter enqueues and `Ctrl+Enter`
sends.

The answer is not a flag kept beside the shortcuts, it *is* the shortcuts.
`setEnterDoes` writes both bindings into the same `S.keys` the handlers read,
so the General pane and the Shortcuts list are one fact shown twice and cannot
disagree — the prompt bar's hint line follows without knowing the setting
exists, because it already read the bindings. That is the same reason the
shortcut list is generated from the registry rather than written out (see 015).

The cost of storing it there is that Shortcuts can still put either chord
anywhere, which is no longer one of the two arrangements. Rather than
silently overwrite that, `enterDoes` reads the pair back as `"custom"`: neither
option is ticked and the pane names the chords actually bound, with picking
either option putting the pair back. A setting that quietly undoes what the
next pane did would be worse than one that admits the state it is in.

The prompt bar's button *is* the selected default, not just its label: it reads
Enqueue and enqueues by default, and Send and sends when the pair is swapped,
each with its own icon. The caret beside it opens the Send menu (see
[012](012-task-queue.md), [028](028-scheduled-jobs.md)), which always lists
Send, Enqueue and Schedule in that order — it does not reshuffle to hide
whichever one is already on the button. It is still one field group, one pill,
one seam at the caret, because the button and its menu are one control. Only
sending is refused while a turn runs, wherever it sits, since enqueueing during
a turn is the point of it.

## One project unfolded, or each keeping its own

`S.foldOthers` is on unless it has been turned off: selecting a project
unfolds it and folds every other one, so a column of eight projects shows one
project's tasks rather than all of them. Turned off, a selection changes which
tasks are numbered and which strip is on screen, and leaves every project
folded or unfolded exactly as it was.

Which way round the default goes is a question about the first launch, so the
key is read as three answers rather than two: an absent key is a window that
has never been asked and keeps the default, and only a stored one turns it
off. Off is written `"0"` — the `""` earlier versions wrote still reads as off,
so a window that answered before the default changed keeps its answer.

It is applied at selection rather than stored as a second flag per project,
which leaves the fold state itself the one fact it already was (see
[042](042-sidebar-project-rows.md)): the chevron and Alt with a project's
letter still fold the selected project on top of the rule, and it stays folded
until it is selected again. A project's chevron also still unfolds it without
folding the selection — only selecting enforces the rule, because that is the
question the setting asks.

What is watched is `S.activeProjectID`, not a call inside `switchProject`. A
project is reached by clicking its row and by `Alt` and its letter, but also by
`Ctrl+PageDown` and by Go to anywhere, which land on a *task* or a *job* and
bring its project with them; one watcher covers all four and cannot be
forgotten by the fifth. Nothing selected folds nothing, so the gap `detachProject` leaves before
the next project is chosen passes through. A project that arrives while the
rule is on arrives folded — `refreshProjects` knows which ids are new — since
it is neither the selected one nor one anybody has opened.

The choice is remembered in `localStorage`, unlike the fold state it acts on:
which projects are folded is where the user is in the list, and this is how
they want the list to behave. Turning it on applies at once, like the colours
in Appearance, because the sidebar it changes is visible behind the dialog.

Both panes count their rows for the rail's filter like the other three, so
`enqueue` narrows to General 2 and Shortcuts 1, and `fold` to General 2.

Pane visibility is applied to a wrapper in the settings dialog, so both
General sections disappear when another sidebar section is selected. Panes
stay mounted to keep filter match counts current.
