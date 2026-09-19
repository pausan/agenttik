# Closing, archiving, restoring and deleting a task

`Ctrl+N` and `Ctrl+T` start a task in the active tab's project.

`Ctrl+W` closes exactly the tab in front — a file, a project page or a task —
and nothing else: files opened from a task stay open, and the task itself stays
active. The tab strip's `×` keeps its cascade instead, closing a task together
with the files opened from it. See [032](032-single-tab-close-shortcut.md).

`Ctrl+Shift+T` restores closed tabs newest-first for the active project, over
as many closes as have been made. Restoring an archived task unarchives it
before reopening its tab.

Archiving a task takes it out of the project's sidebar list and closes its tab
and any files opened from it. It waits in the grey half of the project page's
Tasks tab ([030](030-project-tasks.md)) until it is restored.
Submitting a new prompt, either immediately or to the queue, automatically
unarchives the task and returns it to the project's open task lists. Opening
an archived task to read its transcript leaves it archived.

An untitled task with no transcript, turns, queued work or draft is **deleted**
rather than archived, so glancing at a new task and closing it leaves nothing
behind in the archive or the restore stack.

**Deleting** on purpose works the same way, on any task: a trash icon on every
row of the project page's Tasks and Archived tabs, behind a confirmation
naming the task ([030](030-project-tasks.md)). There is no restore after —
unlike archiving, its turns and messages are gone too — so its tab and any
files opened from it close unsaved rather than asking a second time; the
confirmation already given is the one gate. A running task is stopped first
rather than left to finish into a session that no longer exists.
