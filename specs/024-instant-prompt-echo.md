# Instant prompt echo

## Outcome

Pressing Enter or Ctrl+Enter draws the prompt immediately. The bubble and the
empty box happen on the keypress; a Send also starts the working indicator on
that same turn. Ctrl+Enter keeps its configured Enqueue meaning: its prompt
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
refusal removes it and restores the draft. The prompt is persisted before
queued session title generation, and a sent prompt is persisted before its
first session title, so naming and other follow-up work never comes first.

## Validation

Browser-checked against a real server with the POST held for 1500ms by a route
handler, so client and server latency are told apart. The bubble appeared 19ms
after Enter and the working indicator 27ms after it, both while the request was
still in flight; the reply released at 1514ms left one bubble, not two, and the
turn was really running. With the POST failing 500 instead, the bubble appeared,
then went away, the text was back in the box, and the session was not left
looking busy. `npx vite build` passes.
