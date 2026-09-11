# The project prompt

A project can hold a prompt of its own: standing instructions that every
conversation started in it opens with. It is set on the project page, under
**Prompt**, beside Tasks and Stats. Empty — which is how every project
starts — adds nothing anywhere.

When it is not empty, it goes in front of the **first prompt of each task**,
separated by a blank line, and the task says so: a line above the transcript
reading *Project prompt added*, the text itself on hover, and the project's
Prompt page on click.

## Once per conversation, not once per turn

A turn after the first resumes the provider's own thread, which is still
holding the first prompt — so repeating the injection would only pay for the
same words again. What the agent is told is decided once, at the top of the
conversation, which is also what "standing instructions" means.

The first prompt is the first prompt *the provider is given*, not the first one
in the transcript. A conversation whose opening turn died before the provider
produced anything has that turn erased ([045](045-provider-outage-retry.md)
discards it), so the retry is still its first — and still carries the project
prompt. A rewritten transcript ([014](014-transcript-messages.md)) is the other
way round: the messages go, the turns stay, so an edited opening prompt is not
treated as a fresh conversation and nothing is injected again.

## The copy the task keeps

`sessions.project_prompt` holds what this conversation was given; the project's
own `projects.prompt` is what the next one will be. The copy is taken the
moment a task accepts its first prompt — sent, enqueued, or fired by a schedule
([028](028-scheduled-jobs.md)) — and never rewritten.

Copying rather than looking up is what makes the chip honest. The project text
is editable at any moment, including while a task is mid-answer; without a copy
the transcript would show whatever the project says now over a conversation
that was told something else. It also fixes what a queued prompt will be given
at the point it was accepted rather than the point it runs, which can be an
hour later and several edits away.

Three things stop a task claiming a copy, and all three say the same thing —
this is not the start of a conversation: it already holds one, something has
already been said or is waiting to be said, or a turn has run.

## What the transcript keeps

The user message stored and drawn is the prompt as typed. The injected text is
not a bubble: it is not something anyone said, and a conversation that opens
with a paragraph of standing instructions in its first bubble reads as if the
agent was asked for them. The chip is the whole of its presence in the
transcript, which is why the chip carries the text on hover.

## The chip goes up on the keypress

Both halves of the rule live in two places, and deliberately. The client
applies the same three disqualifications to what it already has on screen and
draws the chip with the echoed bubble, so it appears on the keypress like every
other piece of prompt feedback ([024](024-instant-prompt-echo.md)); the server
applies them to the database and writes the copy. The `started` event carries
the stored session back, which is what settles a disagreement.

## API

`PATCH /api/projects/:id` takes `prompt` alongside `name`, `path` and
`archived`. It is the one field of the four sent as a pointer: an empty prompt
is a real value — it is how the injection is turned off — where an empty name
or path only ever means "not sent".

The project page commits on blur, the same way a schedule's prompt does
([028](028-scheduled-jobs.md)).

## Choices

**On the project page, not in Options.** The name and the folder are one-line
fields and live in the Options pane; a prompt is a paragraph and wants the
width. Putting it on the page also gives the chip somewhere to link to that
shows the text it is talking about — an Options tab the reader would still have
to go and select would not.

**A blank line between it and the prompt.** Nothing heavier: no header, no
markup, no wrapper the agent has to be told to ignore. The provider receives
one prompt that happens to begin with the project's instructions, which every
CLI in [003](003-providers.md) already handles well.

**Archived projects keep theirs.** `ArchivedProjects` does not read the column
because Settings shows a name, a folder and a date; restoring a project brings
its prompt back with everything else under it
([041](041-project-archiving.md)).
