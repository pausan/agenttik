# Reusing an untitled task

New and reused blank tasks inherit the subscription, model, effort and permission
of the newest task in their project, including after a reload. Opening an older
task or using another project does not replace that choice. A removed
subscription falls back to the provider default.

Starting a task lands on an untouched one of that project wherever it is, not
only when it is the tab in front. The chord, the strip's `+` and both New task
buttons all go to it with the cursor in its box. A second empty task is created
only when there is none to go to.

`blankSession` tests every tab of the project against the same three conditions
— nothing said, nothing queued, nothing typed — and prefers the tab in front, so
working from a blank task keeps it. An empty task the user renamed is kept too:
a typed title is a stronger claim on it than a letter in the box.

`blankSessionID` continues the search into the tasks with no tab open. It reads
the project's tasks, archived ones excluded, and takes the newest with no title,
nothing queued and no draft kept for it. The server names a task from its first
prompt, sent or queued, so an untitled one with nothing kept has an empty
transcript and no text to lose: reopening it is what starting a task would have
produced anyway. One still holding an unsent prompt ([004](004-ui.md#drafts)) is
left alone, exactly as the open tab holding one is. A list request that fails is
not reported — creating a task is still correct.

Started from a project page, reuse hands that tab over exactly as creating did,
so the same button leaves the same strip either way.

The cursor is asked for once the switch has been drawn. `focusPrompt` increments
a counter the prompt bar watches, and a watcher only sees requests made after it
mounts — arriving from a project page mounts the bar with the very switch that
asks for focus. `await nextTick()` between the two puts the request after the
mount; without it the reused task came to the front with focus left on the
button that was clicked.

## Choices

**The queue is checked as well as the title**, though naming a task happens
before its first prompt enters the queue. It is one term, and it keeps a task
whose title write failed from being handed over with a prompt waiting in it.

**Only the project's own tasks.** An empty task belonging to another project
would be the wrong place to type: the agent would run in the wrong folder.
