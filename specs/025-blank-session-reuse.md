# Reusing an untitled conversation

## Outcome

Starting a session lands on an untouched conversation of the project wherever
it is, not only when that conversation is the one in front. The chord, the
strip's `+` and both New session buttons go to it with the cursor in its box.
A second empty conversation is created only when there is none to go to.

`blankSession` read `S.owner` alone, so having a used conversation selected was
enough to start an empty one beside the empty one already open. It now tests
every tab of the project against the same conditions — nothing said, nothing
queued, nothing typed — and prefers the tab in front, so working from a blank
conversation keeps it. An empty session the user renamed is kept too: a typed
title is a stronger claim on it than a letter in the box.

`blankSessionID` continues the search into the conversations with no tab open,
which is what the sidebar's `Untitled session` rows are. It reads the project's
sessions, archived ones excluded, and takes the newest with no title and
nothing queued. The server names a session from its first prompt, sent or
queued, so an untitled one has an empty transcript and no draft to lose:
reopening it is what starting a session would have produced anyway. A list
request that fails is not reported — creating a session is still correct.

Started from a project page, reuse hands that tab over exactly as creating did,
so the same button leaves the same strip either way.

The cursor is asked for once the switch has been drawn. `focusPrompt`
increments a counter the prompt bar watches, and a watcher only sees requests
made after it mounts — arriving from a project page mounts the bar with the
very switch that asks for focus. `await nextTick()` between the two puts the
request after the mount; without it the reused conversation came to the front
with focus left on the button that was clicked.

## Choices

**The queue is checked as well as the title**, though naming a session happens
before its first prompt enters the queue. It is one term, and it keeps a
session whose title write failed from being handed over with a prompt waiting
in it.

**Only the project's own conversations.** An empty session belonging to another
project would be the wrong place to type: the agent would run in the wrong
folder.

## Validation

Browser-checked on a fresh database against this repo, with no console or page
errors, driving `Ctrl+N` and the project page's button.

With a used conversation selected and an empty one open in another tab,
`Ctrl+N` left the session count at 2 and brought the empty tab to the front
with the textarea focused. Closing that tab and pressing `Ctrl+N` from the used
conversation reopened the same session — count still 2 — rather than creating a
third. Typing a letter into it and pressing `Ctrl+N` created one, count 3, and
left the draft where it was. From the project page, New session reused the
empty tab, closed the page's own tab, and focused the box.

`make ui` passes.
