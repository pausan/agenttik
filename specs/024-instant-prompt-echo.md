# Instant prompt echo

Pressing Enter or Ctrl+Enter draws the prompt immediately. The bubble and the
empty box happen on the keypress; a Send also starts the working indicator on
that same turn. Enter keeps its configured Enqueue meaning: its prompt
appears as a pending dashed row immediately, then becomes the server-backed
queued row or a normal transcript bubble if the queue starts at once.

## Priority

> instant user feedback along with performance is of upmost importance

Sending used to clear the box first and draw the prompt only once the server
had answered. The answer is not slow, but everything between the keypress and
it — a session read, three SQLite writes and a CLI launch — is time the
transcript spent empty, and none of it changes what the bubble says. Your own
text is the one thing the client already knows.

Three details keep the optimistic bubble honest:

- **It is not drawn twice.** Both the POST reply and the `started` event that
  races it go through `startLocal`, which skips a prompt that is already the
  last message; the reply now passes no prompt at all, since the bubble it
  would add is up.
- **A refusal takes it back.** `push` returns the message it added, so the
  failure path removes that exact one rather than the last, and returns the
  text to the box — unless something has been typed since, which is the next
  prompt and not this one.
- **`running` is restored, not cleared.** The old failure path set it to false
  unconditionally, which hid the working indicator of a turn that was still
  going whenever a second prompt was refused as busy. It now goes back to what
  it was.

Marking the session running on the keypress also disables Send, so a second
Enter during the round trip cannot start a second turn.

The pending queue row has no server id, so its controls stay disabled until the
response replaces it. A started event claims the pending row directly, while a
refusal removes it and restores the draft. Both prompts are persisted before
the task is named, and naming itself only writes the prompt's first line and
sends the real request to the background, so no follow-up work comes first.
