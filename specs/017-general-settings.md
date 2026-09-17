# General settings

Settings grew a fourth section, **General**, and it is where the dialog now
opens: the sidebar's Settings button and the launcher's Settings entry both
land there, the keyboard button still lands on Shortcuts.

It controls prompt submission, project folding, where new tasks and projects
are inserted, and project task search. **Fuzzy Search** is the default;
**Smart Search** downloads Bekko a8m and indexes tasks locally, with download
and indexing progress in this pane. See [057](057-smart-search.md).

## Desktop tray

Desktop launches offer **Close to tray** and a configurable global show/hide
shortcut, defaulting to `Ctrl+Shift+A`. Save and restart to apply changes.
See [060](060-desktop-tray.md) for lifecycle and platform support.

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
`Ctrl+PageDown` and by Command Palette, which land on a *task* or a *job* and
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

Pane visibility is applied to a wrapper in the settings dialog, so all
General sections disappear when another sidebar section is selected. Panes
stay mounted to keep filter match counts current.

## New tasks and projects

New tasks and projects are inserted at the top by default. General offers
**At the top (default)** and **At the bottom**. The choice applies to future
inserts; existing items retain their saved order, including manual drags.
The orchestrator project remains pinned above ordinary projects.

The choice lives in SQLite's singleton `general_config` row, exposed through
`GET /api/general` and `PUT /api/general` as `new_item_position` (`top` or
`bottom`). It survives restarts and applies to all windows and creation paths,
including scheduled tasks. Each insert selects `MIN(position) - 1` for top
or `MAX(position) + 1` for bottom in the same SQL statement. Task positions
are scoped to their project; archived items count toward both bounds.
Sidebar and project task lists share these positions. Schedule rows and
archive history keep their own ordering.

## Task sounds

**Play task sounds** is off by default. When enabled, this window plays a short
synthesized ding on a live final turn completion or error, including tasks without
an open tab. An error and its completion sound only once per turn; duplicate
project/session deliveries are ignored. Loading history does not play sounds.
The choice is stored as `agenttik.taskSounds` in browser/profile preferences and
Reset defaults turns it off. The filter matches sound, ding, notifications,
completion, attention, and error. Audio is initialized only when enabled and
unlocked by a click or key press; browser autoplay restrictions can keep it silent
until that interaction. There is no interactive approval bridge in the task runner.

## Database

Database appears in General. It shows the absolute database path
on the Agenttik host as text over a gray background, with a button that copies
the full path and confirms success with a check. The file size uses decimal
units (B, KB, MB, and so on), rounded to one decimal. It measures the main
`.db` file only, excluding SQLite's WAL and shared-memory files. Details are
read when the settings pane mounts, without polling.
`GET /api/general` includes `database_path` and `database_size` (bytes);
these fields are informational and cannot be changed through `PUT /api/general`.
The settings filter matches database, path, size, storage, and copy.

## Reset defaults

The final section offers a warning dialog with Cancel and Reset defaults.
Confirmation restores blue/zinc colors, System mode, keyboard shortcuts, prompt
submission, folding, layout widths, file/diff modes, task list choices, remembered
model and schedule choices, and fuzzy search in this browser. It clears model
favourites and visibility overrides, resets default subscription choices, restores
top insertion and default tray settings for all windows. Tray changes need a restart.
`POST /api/general/reset` updates these database preferences in one transaction.
Only named browser preference keys are removed; authentication, accounts, server
configuration, project/task data, open tabs, files and drafts are preserved.
The reset applies in place without reloading the page.
