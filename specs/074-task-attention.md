# Task attention

Sidebar and project task rows show a bold title and steady amber dot when a
turn finishes and its result has not been read. Failed tasks keep their red
status dot, including after reading. Running tasks keep their activity dot;
provider retry waiting keeps its existing amber status after acknowledgement.

Project rows in the sidebar show a steady amber dot when any of their tasks
has an amber unread dot, including manually marked unread tasks. The amber
dot pulses while a task or scheduled job in that project is running. Without
unread task attention, running projects keep the primary activity pulse and
inactive projects have no dot. Folding a project does not hide its indicator.

Reading means choosing the task transcript and keeping it selected in a
visible window for three continuous seconds. Clicking and keyboard navigation
behave alike. Leaving the transcript or hiding the window cancels the timer.
Reading clears the bold title and returns the dot to its normal status.

Only a visit the user chose reads. A completion that arrives while the task
is already selected stays unread until the user leaves and returns, so a run
left alone in front is still unread when the user comes back. A transcript
the app selects by itself — tabs restored on load or profile return, or the
neighbour shown after closing a tab — is held the same way.

The store records final persisted `done` stream events, including tasks with
no open tab. A new turn clears the previous unread result. One Vue watcher
owns one timeout for the selected transcript; row count adds no timers.
The task's status dot reads an unmarked task a finished turn left unread,
marks a read, unmarked task unread, then cycles its
mark; see [098](098-task-flags.md). A marked task shows its mark in place of the dot and
keeps the bold title while unread.
Right-clicking a sidebar or project task row opens actions for opening, renaming,
and marking read or unread, plus stop, archive/unarchive, scheduled job, and
project-list deletion when those controls are available. Right-clicking does
not select the task; deletion keeps the existing confirmation. During inline
rename the text field keeps its native context menu.

Mark unread works even without a completed turn. On the selected task it holds
until leaving and returning to the transcript, like a completion; hiding the
window alone does not clear it. A new turn or completion replaces that manual
state. Manual unread uses `-1` alongside positive unread turn IDs.

Unread markers are saved through instance/profile-scoped UI storage and
restored on reload. The client also saves each known task's latest completed
turn ID. Project task rows report `last_completed_turn_id`, read using the
existing session/turn index. Refreshing the project list compares those IDs
and marks newer results unread, including turns that finished while another
profile was open or the stream was disconnected. Initial history establishes
a baseline without marking old results unread. Acknowledgement keeps that
baseline, so reloads do not mark an already-read completion unread again.
Results superseded by a running turn are skipped. Older remote servers without
completion IDs keep live-event behaviour. Private mode uses memory storage.
Read state is local to this client.

`task-attention.test.js` checks interrupted visits, hidden windows, completion
during viewing, app-made selections, missed completions, and persistent error
coloring. Browser tests cover menu toggles without selecting the task,
automatic reading after opening a task, and a result finishing on the
selected task. They also switch profiles during selected and unselected tasks,
check that the restored transcript stays unread, check persistence on return
and reload, then read and reload the result.
