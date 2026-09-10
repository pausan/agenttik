# Collapsing a run of tool calls

## Outcome

Consecutive tool calls in a transcript are drawn as one block that shows only
its latest call. Above it sits a button reading how many the run holds — "6
tool calls" — which opens all of them and closes them again. A run of a single
call has nothing to hide and is drawn as the plain bubble it always was.

A turn routinely spends a dozen calls reaching one answer, and the reply is
what the transcript is read for. Showing the newest call keeps what the agent
is doing right now on screen while the turn runs, and keeps the reply within a
screen of the prompt once it has finished.

The state is per run and per session tab: opening one run leaves the others
closed, and a run stays open while it grows, so watching a long turn does not
close under you. Anything other than a tool message — a reply, a thought, an
error — ends the run.

## Where it lives

`ToolGroup.vue` owns the button and the open flag. `Transcript.vue` groups the
messages: one pass over the list emitting either a message or a run of them.

Grouping reads `role` and nothing else, which is what keeps it off the
streaming path — a delta only grows a message's `content`, so the computed
that builds the rows is not invalidated and the bubble that changed is still
the only one that re-renders. Rows are keyed by the position the run starts
at, so a group keeps its component, and its open flag, as calls arrive.

## Validation

`npm run build` passes. Browser-checked against a live session whose CLI
streamed a run of six tool calls, a reply, then a lone call: collapsed showed
the sixth call only and one toggle, expanding showed all six, collapsing
returned to one, the lone call had no button, and `aria-expanded` tracked the
state. No page or console errors.
