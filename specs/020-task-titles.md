# Task titles

The first prompt of an untitled task names it, whether it is sent or queued.

Naming never holds up the task. The prompt's first line, trimmed to 80
characters, is written as the title before the turn starts, so no row is ever
drawn as `Untitled task` and the keypress is not waiting on a provider. A
better title is fetched behind it and replaces the first line when it arrives,
usually a few seconds later.

The selected provider supplies its lightest advertised model: Codex uses
GPT-5.6 Luna with no reasoning and Claude Code uses Haiku with low effort.
The one-shot request runs read-only from the system temporary directory and
receives only the prompt to title. It has no project files, transcript, real
session ID, or provider session ID, so it cannot become part of the actual
task. An 8-second limit or any provider failure leaves the first line
standing.

The request asks for the intent behind the prompt rather than a trim of its
wording: an action verb, a subject named from the request itself, and none of
the greetings, background, logs, or diffs a prompt is written with. Left to
itself a small model echoes the opening words, which is what made first lines
and generated titles hard to tell apart.

**The generated title only replaces the placeholder.** The update is
conditional on the row still holding the first line that was written for it,
so renaming the task by hand while the request is in flight keeps the name the
user chose. A provider that returns the placeholder unchanged writes nothing.

## Reaching the screen

The replacement is published as a `session_titled` event on both the session's
topic and its project's, carrying the session row. The sidebar, the project
lists and the tab strip all draw the title, and a task with no row on screen is
in neither list that would otherwise carry the new name back, so the open tab is
relabelled from the event itself.
