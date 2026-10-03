# Transcript navigation, copying and editing

Up and Down arrows sit outside the prompt box, vertically centered beside its
right edge, for scrolling through human messages. Up stops at the first human
message. Down stops at each later human message, then goes to the end of the
transcript; further presses do nothing.
From the end, Up goes to the last human message. Manual scrolling resets the
navigation position to the current viewport. Task switches reset the navigation
cursor. Queued prompts and tool messages are not human-message stops.
Navigating to a human message pauses following live output; navigating to the
end resumes it. The prompt draft is unchanged.

While reading replies, the most recent human prompt above the viewport stays
at the top in a compact bubble once its original bubble is fully out of view.
Scrolling through older turns shows the prompt for that part of the transcript.
Queued prompts are excluded. The copy preserves plain text and line breaks,
shows at most three wrapped lines, and places `… Show more` at the end of the
last visible line when clipped. A short fade separates the text from the label;
the label adds no extra line or bubble height.
Clicking it scrolls to the full original prompt and pauses following live output.
That prompt stays unpinned until its bottom is half a viewport above the top.
Task switches reset this dismissal. The overlay does not shift transcript rows
or saved scroll positions. Human nodes are cached when rows change; scrolling
uses a binary search, and resize observations handle wrapping and growing output.

Every message in a transcript can be copied. Hovering a bubble reveals the
actions beside its role label; they occupy the layout at all times, so nothing
shifts when the pointer arrives. The copy icon becomes a tick for a moment so
the click is visibly acknowledged.

Your own prompts also get a pencil. It opens the prompt for editing in place —
where it sits in the transcript, not in a dialogue over it — with Send,
Enqueue, the shared model control, Cancel, and Escape to leave. The control has
the same fuzzy model picker and separate effort select as the prompt bar,
without its favourite button. It updates the task's choice. Sending
truncates the transcript at that message and sends the new text as an ordinary
turn. Enqueue truncates at the same point and queues the replacement with the
selected model; it waits for other running tasks in the project. Both actions
are available after a failed or stopped turn, once the task is no longer running.

Editing is offered only for a `user` message that has been stored. A message
still arriving over the stream is built locally and has no id to rewrite, so
the transcript is re-read from the store when a turn ends — which is also what
reconciles anything the stream and the store disagree about.
After an edit is accepted, the transcript, queue and running state are read
back together. A fast retry that finishes before the edit request returns
therefore stays finished in the UI.

A prompt that has not started yet is edited the same way but is not one of
these messages — it is still in the queue; see
[012](012-task-queue.md#editing-a-queued-item).

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
| POST | `/api/sessions/:id/messages/:message/edit` | `202` and the new turn, or `{queued, queue_count}` when `enqueue: true` |

The message must belong to the session named in the path and must be a `user`
message; an id alone says nothing about either. A session with a turn in
flight is refused *before* anything is deleted, so a busy session cannot lose
the tail of its transcript and get no turn for it. Deletion is `id >= from`:
ids are handed out in order, so that is "from here down".
