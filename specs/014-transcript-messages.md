# Copying and editing transcript messages

## Outcome

Every message in a transcript can be copied. Hovering a bubble reveals the
actions beside its role label; they occupy the layout at all times, so nothing
shifts when the pointer arrives. The copy icon becomes a tick for a moment so
the click is visibly acknowledged.

Your own prompts also get a pencil. It opens the prompt for editing in place —
where it sits in the transcript, not in a dialogue over it — with Send,
Cancel, and Escape to leave. Sending truncates the transcript at that message
and sends the new text as an ordinary turn.

Editing is offered only for a `user` message that has been stored. A message
still arriving over the stream is built locally and has no id to rewrite, so
the transcript is re-read from the store when a turn ends — which is also what
reconciles anything the stream and the store disagree about.

## What editing does not do

Two limits, both stated in the editor rather than discovered afterwards:

- **The agent is not rewound.** Continuity is the provider's own session id,
  and neither CLI can truncate its history — `claude --resume` has no rewind,
  and `--fork-session` copies a history rather than cutting one. So the agent
  still remembers the exchange that left our transcript, and reads the edit as
  "I meant this instead". That is the useful half of the intent; pretending
  otherwise would mean starting a fresh provider session, which forgets
  everything *before* the edit too.
- **Nothing on disk is reverted.** The files are whatever the earlier turns
  left behind.

The turns themselves are also left alone. They are what the run actually cost,
and rewriting a transcript does not unspend it — so Stats still totals the
work that was done, and only the transcript is shorter.

## HTTP

| Method | Path | Answers |
|---|---|---|
| POST | `/api/sessions/:id/messages/:message/edit` | `202` and the new turn |

The message must belong to the session named in the path and must be a `user`
message; an id alone says nothing about either. A session with a turn in
flight is refused *before* anything is deleted, so a busy session cannot lose
the tail of its transcript and get no turn for it. Deletion is `id >= from`:
ids are handed out in order, so that is "from here down".

## Validation

`go build ./...`, `go vet ./...` and `npm run build` pass. Against a live
haiku session: editing an assistant message is refused ("only your own prompts
can be edited"), an id from another session is a 404, an empty prompt is a
400, and a real edit left the transcript as the rewritten prompt and its new
reply with both turns still recorded. Browser-checked: the You bubble offers
Copy and Edit, the Agent bubble only Copy, the clipboard read back the agent's
text, the editor opened prefilled with the existing prompt and its caveat
line, and Escape closed it without sending. No page or console errors.
