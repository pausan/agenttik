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

Reading means keeping the task transcript selected in a visible window for
three continuous seconds. Clicking and keyboard navigation behave alike.
Leaving the transcript or hiding the window cancels the timer. A completion
that arrives while reading starts a fresh three seconds. Reading clears the
bold title and returns the dot to its normal status.

The store records final persisted `done` stream events, including tasks with
no open tab. A new turn clears the previous unread result. One Vue watcher
owns one timeout for the selected transcript; row count adds no timers.
Right-clicking a sidebar or project task row opens actions for opening, renaming,
and marking read or unread, plus stop, archive/unarchive, scheduled job, and
project-list deletion when those controls are available. Right-clicking does
not select the task; deletion keeps the existing confirmation. During inline
rename the text field keeps its native context menu.

Mark unread works even without a completed turn. On the selected task it holds
until leaving and returning to the transcript; hiding the window alone does
not clear it. A new turn or completion replaces that manual state. Manual
unread uses `-1` alongside positive unread turn IDs.

Unread markers are saved through instance/profile-scoped UI storage and
restored on reload. Private mode uses memory storage. Read state is local to
this client; completions missed while disconnected are not backfilled.

`task-attention.test.js` checks interrupted visits, hidden windows, new results,
completion during viewing, and persistent error coloring.
