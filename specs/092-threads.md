# Threads

Selecting text inside one stored message of a conversation shows **Ask in
thread** under the selection. It opens a box under that message, with the
passage quoted; the question is answered in a thread drawn there, indented,
as a small chat with its own follow-up box. The conversation itself does not
change: no bubble, no turn, nothing in its provider history.

Question and reply boxes start at two rows, with larger controls and 16 px
padding. Quotes and closed thread chips use 14 px text; mobile panels use the
available width without an extra indent.

## Image areas

Attached images and image outputs open `ConversationImage.vue`, a large
zoomable dialog. **Circle an area** switches to a fitted image with a drag
surface (`ImageArea.vue`). Mouse, pen and touch drags draw an ellipse; reverse
drags work, drags outside the picture clamp to its edges, and areas smaller
than 8 screen pixels in either dimension are discarded. Dragging again
replaces the selection. Switching back restores the zoomable viewer.
The ask action handles touch release directly and suppresses the synthetic
click, which browsers can discard after drawing. Lifting a finger outside
the action cancels it.

**Ask in thread** saves a copy of the whole image with a red circle and opens
the thread question form under its stored message. The form shows the marked
copy before sending. The quote names the source and bounds in original image
pixels. The question carries the copy as an ordinary attachment, so the runner
resolves it to a local image file for the provider's image-reading tool
([056](056-pasted-images.md)). The marked image remains in the thread's first
message across reloads. The original image stays intact. Images inside threads
can be viewed and zoomed, but have no nested thread action.

`image-area.js` keeps coordinates relative to the image, bounds exported
canvases to 2048 px on the longer edge, and draws a white outline behind the
red circle. Copies use PNG, with JPEG fallback when PNG exceeds the 4 MiB
attachment limit. An upload or encoding failure leaves the viewer and
selection open with an error. Animated images produce a still copy.

A closed thread is a chip under its message: its title (the first line of its
first question) and how many replies it has, spinning while it works. A
thread can be closed, deleted (the button asks twice) or **opened as a
task**.

## Data

A thread is a session with `parent_id` set to its conversation,
`anchor_message_id` and `anchor_quote` (cut at 4,000 bytes) naming the
passage. It copies the conversation's provider, account, model, effort and
permission. Being a session, it reuses the runner, streaming, approvals,
stopping and stats. `ListSessions`, the sidebar's recent sessions and the
project's task count leave threads out; the session detail lists them as
`threads` (`ThreadRef`: id, title, status, anchor, reply count). A thread
cannot have threads of its own.

Deleting a conversation stops and deletes its threads. Editing a prompt
deletes the threads quoting the messages the edit removes, along with them.

## Context

A thread starts from the conversation as it is when its first question is
sent:

- A provider that is an `agent.Forker` starts the thread's first turn as a
  copy of the conversation (`TurnRequest.ForkFrom`). Claude Code runs
  `--resume <conversation> --fork-session`, which keeps the tool results the
  agent saw and leaves the original untouched; the CLI picks the copy's id
  and reports it as the turn starts. The project prompt is already in the
  copy, so it is not added again.
- Any other provider, or a conversation on another provider or account, or
  one that has not reached its provider yet, is given the transcript as text
  (user and agent messages, the last 60,000 bytes) in front of the first
  prompt.

Either way the first prompt also carries the passage, in a `<thread>` block
saying it is a side thread. The thread shows only the question. Later turns
resume the thread's own provider session.

## Opening as a task

**Open as task** (`POST /api/sessions/:id/spawn`, refused while the thread is
working) creates an ordinary task in the same project with the thread's
title and choices, `origin_id` naming the thread, and a copy of the thread's
messages without its turns, so cost stays where it was spent. Its first turn
inherits as a thread does, from the thread: a fork of the thread's provider
session, or else the conversation, the passage and the thread as text. The
thread stays under its message. The task's transcript starts with
"Continued from a thread in …", which opens that conversation with the
thread open.

## Client

`S.threads` holds open threads by session id. Each is shaped like a session
tab (`sessionID`, `detail`, `draft`), so the stream subscription includes
them and `onSessionEvent`, `syncMessages` and the bubbles serve them
unchanged. A thread's turns mark no task unread; when one ends, the parent's
detail is re-read for its reply count. Thread bubbles cannot be edited.
`S.threadDraft` holds the quote and optional marked image attachment while
its first question is written. `createThread` adds that attachment to the
question with `withImages`; the HTTP shape remains the same.

## HTTP

| Method | Path | Does |
|---|---|---|
| POST | `/api/sessions/:id/threads` | `{message_id, quote, prompt}` → `{thread, turn}`; sends the first question |
| POST | `/api/sessions/:id/spawn` | a thread → the new task |
| POST | `/api/sessions/:id/messages` | a follow-up, as for any session |
| DELETE | `/api/sessions/:id` | deletes a thread, or a conversation and its threads |

## Tests

Runner tests cover forking on the first turn only, the passage preamble, the
parent's provider session staying put, a spawned task forking the thread, and
the transcript fallback for both. Server tests cover validation, listing,
reply counts, threads staying out of task lists, spawning, origin, edits and
deletes. A browser test selects text, asks, follows up, reloads, opens the
thread as a task, sends there and follows the link back. Image tests check
relative coordinates, reverse drags, bounded canvas sizes, the actual uploaded
circle pixels, provider attachment resolution, reloads, and SVG output selection.
