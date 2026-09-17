# Task attention

Sidebar and project task rows show a bold title and steady amber dot when a
turn finishes and its result has not been read. Failed tasks keep their red
status dot, including after reading. Running tasks keep their activity dot;
provider retry waiting keeps its existing amber status after acknowledgement.

Reading means keeping the task transcript selected in a visible window for
three continuous seconds. Clicking and keyboard navigation behave alike.
Leaving the transcript or hiding the window cancels the timer. A completion
that arrives while reading starts a fresh three seconds. Reading clears the
bold title and returns the dot to its normal status.

The store records final persisted `done` stream events, including tasks with
no open tab. A new turn clears the previous unread result. One Vue watcher
owns one timeout for the selected transcript; row count adds no timers.
Unread turn IDs are saved through instance/profile-scoped UI storage and
restored on reload. Private mode uses memory storage. Read state is local to
this client; completions missed while disconnected are not backfilled.

`task-attention.test.js` checks interrupted visits, hidden windows, new results,
completion during viewing, and persistent error coloring.
