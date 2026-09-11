# Task outcomes

An archived task says what it came to. Under its row in the project's task
list, below the title and in the lighter, italic voice the transcript gives
thinking, sits one sentence: *Removed the sleep from the queue scheduler test*.
The row above it says what was asked; this says what happened.

It is written when the task is archived, from the last thing the agent said in
it. That is where an agent puts its closing account of the work — prose is
flushed to the transcript before every tool call, so the newest assistant
message is the end of the final turn rather than the whole of it — and it
costs nothing to reach: one indexed read of a table the task already filled.

The provider's lightest model turns that reply into the sentence, the same
model and the same isolated one-shot request that names a task from its first
prompt ([020](020-task-titles.md)). Claude Code uses Haiku at low effort,
Codex GPT-5.6 Luna with no reasoning, Copilot the cheapest model its account
is entitled to.

**Nothing waits for it.** Archiving is a click: the row goes grey, leaves the
sidebar and moves to the history at once, and the sentence arrives a few
seconds later on the `session_summarized` event, which carries the session so
the list redraws the row rather than re-reading it. A request that times out
after 15 seconds, a provider that offers no small model, a task archived
before it ever ran — each leaves the line blank, and a row without one is
simply a row with one line fewer. Every task archived before this existed is
blank for the same reason and stays that way: there is no backfill.

Both ways a task is archived write one. The archive button on any of its rows,
and a scheduled run closing itself when its turn ends
([028](028-scheduled-jobs.md)) — a job's run is a task that finished, and
forty runs of one job are exactly the list where *which of these did
anything* is worth answering without opening each.

## Choices

**Archiving is the trigger, not the end of a turn.** A turn ending means the
agent stopped talking, which happens several times in a conversation that is
still going. Archiving is the one moment someone says the work is over, and it
is also the moment the row stops being something to open and starts being
something to scan.

**Written again each time, not once.** A task pulled back out of the archive,
worked on further and archived again is summarised again, against whatever it
says by then. A stale sentence is worse than none: it describes work that has
since moved on, and nothing on the row says how old it is.

**Restoring hides it rather than clearing it.** Only archived rows draw the
line, so a restored task shows nothing — its summary is still in the column,
about to be replaced the next time it is archived. And an answer that lands
after a restore is dropped rather than stored: the session is read again
before the write, and a task back in the working list is not a finished one to
describe.

**One line, capped at 160 characters, clamped to two on screen.** The request
asks for 6–18 words; the cap is what stops a model that ignores that from
turning a list of 25 rows into a page. The reply going in is capped too, at
4000 characters — a long reply is mostly its middle, and both halves are
tokens spent on every archive.

**`sessions.summary`, a column like the rest.** Blank is the whole of "not
summarised", which covers the backlog, the failures and the tasks that never
ran without a second flag to read. `SetSessionSummary` deliberately leaves
`last_active_at` alone, for the same reason archiving does: this is written
*about* the task, not *by* it, and the archive is ordered by that clock.

**The question is asked of the reply, not of the transcript.** A whole
conversation would be the honest input and is not worth what it costs: a
summary would then be charged per turn of history on every archive, and the
closing message of a task is written to be exactly this — the agent's own
account of what it did.

**`agent.SmallModel` rather than `TitleGenerator`.** The interface that names
a provider's lightest model now answers two questions about a task instead of
one, so it is named for what it supplies and not for the first thing that was
asked of it. `Runner.askSmall` holds the isolation rules — a fresh read-only
request in the system temporary directory, with no session id, transcript or
project files — so both callers get them without either repeating them.
