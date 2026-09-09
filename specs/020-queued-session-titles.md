# Queued session titles

## Status

Implemented.

## Behaviour

The first queued prompt names an untitled session before queue scheduling.
This means a session waiting behind another project turn does not appear as
`Untitled session` in the sidebar.

The selected provider supplies its lightest advertised model: Codex uses
GPT-5.6 Luna with no reasoning and Claude Code uses Haiku with low effort.
The one-shot request runs read-only from the system temporary directory and
receives only the prompt to title. It has no project files, transcript, real
session ID, or provider session ID, so it cannot become part of the actual
conversation. An 8-second limit or any provider failure falls back to the
first line of the prompt.
