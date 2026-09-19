# Direct API task runtime

`agent/direct` runs a bounded local coding loop for OpenCode Go and API-key
providers. It supports streamed Chat Completions and native Anthropic Messages,
including fragmented tool calls, reasoning/signature continuation data, tool
results, token accounting and cancellation. HTTP errors never include raw
provider response bodies. A truncated or rejected response fails the turn.

## Instructions and tools

Each task detects installed shells through `PATH` and checks `python3`/`python`
for Python 3 with a one-second version probe. The system prompt names the host,
project, permissions and detected executables. It loads the project's root
`AGENTS.md` (up to 64 KiB), tells the model to check more specific instructions
before editing subdirectories, complete the requested work, verify results,
iterate on errors, and finish with a concise summary and any limitations.
Ordinary file/tool content is treated as data. Isolated metadata requests have
no tools or repository instructions; isolated repository operations have tools
but do not inherit AGENTS.md.

| Permission | Tools |
|---|---|
| Plan | `read_file`, `list_directory` |
| Workspace | Plan plus `write_file`, including missing parent directories |
| Full | Workspace plus detected `shell` and `python` |

File tools use Go's `os.Root` to confine paths and symlinks to the project.
Full commands execute on the host in the project directory; this is not a
shell sandbox. Shell selection is restricted to detected executables. Processes
are killed as a group on cancellation or after 60 seconds. Tools enforce the
permission at execution time, even if the model requests an unadvertised tool.
Credentials are supplied only to HTTP requests, not added to tool environments.

There are at most 64 model steps per turn, 64 tool calls per response, 64 KiB
per read/output, 1 MiB per write and 1,000 directory entries per listing.
HTTP requests have a five-minute timeout and a 16 MiB response/history limit.
Anthropic responses have an 8,192-token output limit; Chat Completions uses the
provider's default output budget. Provider-side spending limits remain useful.

## Continuation

Private `direct-<uuid>.json` files store the working folder and messages.
Resume requires the same provider/account history directory and working folder.
The current prompt and completed tools are saved even when a later request
fails or is cancelled, so a follow-up has the results of changes already made.
History is written atomically with mode 0600 on POSIX. Isolated requests do
not load or save history. There is no automatic context compaction; exhausted
model context requires a new task. Interrupted response fragments are not saved.

This is a basic text coding runtime. It does not provide image understanding,
MCP, plugins, subagents, interactive approvals or a local currency budget.

## Verification

Local HTTP/process fixtures cover native Anthropic and Chat Completions tool
loops, continuation metadata, resume, failed-turn history, malformed streams,
usage, permissions, traversal/symlinks, executable detection and cancellation.
No live paid API request is part of these tests.
