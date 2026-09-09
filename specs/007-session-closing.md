# Session closing and reopening

## Outcome

`Ctrl+N` and `Ctrl+T` start a session for the active tab's project. Closing a
session through its tab close button or `Ctrl+W` deletes it when its transcript
has no messages. A non-empty session only loses its tab; it is neither archived
nor changed on the server.

The browser keeps a most-recent-first stack of non-empty session ids closed in
that window. `Ctrl+Shift+T` consumes the newest id and opens that session again,
so repeated presses restore older closed sessions one by one. The stack is not
persisted across a reload.

## Validation

No tests were run for this prototype change, as requested.
