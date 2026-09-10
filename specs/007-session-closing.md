# Session closing and reopening

## Outcome

`Ctrl+N` and `Ctrl+T` start a session for the active tab's project. `Ctrl+W`
closes exactly the tab in front, including a file or project tab, and leaves
files opened from a session in place; the session remains active. The tab
strip's `×` still closes a session and its owned files together.
`Ctrl+Shift+T` restores closed entries newest-first for the
active project, including more than one close. Archiving a session removes it
from the project sidebar and closes any open tab and owned files. Restoring
that archived entry unarchives the session before reopening its tab. An
untitled session with no transcript, turns, queued work, or draft is deleted
when archived instead, so it does not appear in the Sessions list or restore
stack.

## Validation

No tests were run for this prototype change, as requested.
