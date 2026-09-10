# Instant prompt echo

## Outcome

Pressing Enter draws the prompt immediately. The bubble, the empty box and the
working indicator all happen on the keypress; the request to start the turn
goes out behind them.

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

Enqueue is unchanged: its bubble needs the queue row's id, which only the
server can give it.

## Validation

Browser-checked against a real server with the POST held for 1500ms by a route
handler, so client and server latency are told apart. The bubble appeared 19ms
after Enter and the working indicator 27ms after it, both while the request was
still in flight; the reply released at 1514ms left one bubble, not two, and the
turn was really running. With the POST failing 500 instead, the bubble appeared,
then went away, the text was back in the box, and the session was not left
looking busy. `npx vite build` passes.
