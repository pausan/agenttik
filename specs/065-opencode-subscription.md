# OpenCode Go and subscription access policy

## Provider policy review

Reviewed 2026-09-15 against provider documentation:

| Subscription | Integration in agenttik | Source |
|---|---|---|
| Claude | Claude Code CLI required. No direct OAuth login or credential reuse. | [Anthropic legal and compliance](https://code.claude.com/docs/en/legal-and-compliance) prohibits third-party use of subscription OAuth credentials. |
| ChatGPT / Codex | Codex CLI required by the supported integration. | [Authentication](https://developers.openai.com/codex/auth) and [SDK](https://developers.openai.com/codex/sdk) document subscription login through Codex and CLI-backed custom clients. |
| GitHub Copilot | Copilot CLI required by the supported integration. | [SDK quickstart](https://docs.github.com/en/copilot/get-started/sdk-quickstart) requires the CLI runtime; some SDKs bundle it. |
| OpenCode Go | CLI optional; direct coding-agent requests supported. | [Go](https://opencode.ai/docs/go/#where-can-i-use-it) documents third-party coding agents, their identification and conversation headers. |

The Codex and Copilot requirements describe the documented integrations used
here, not a claim that their policies explicitly prohibit every alternative.
agenttik does not extract their tokens or impersonate a first-party OAuth
client. Bundling a CLI would remove a separate installation step but would
still run the CLI; this build does not bundle runtimes.

## Account and execution choice

Provider `opencode` means **OpenCode Go**, the subscription. Zen pay-as-you-go
billing is not a separate provider. Go's own console can enable spending Zen
balance beyond its allowance; agenttik does not change that setting.

Settings offers `auto` (default), `direct`, and `cli` per account. Automatic
selects an installed `opencode` for new conversations, otherwise direct HTTP.
Existing conversations keep their backend in automatic mode. Explicitly
switching backend starts a new provider conversation on the next prompt.
A failing CLI never silently falls back to direct execution.

The login folder is an XDG data root; credentials live in
`<home>/opencode/auth.json` under `opencode-go`, in OpenCode's API-key format.
System uses `XDG_DATA_HOME` or `~/.local/share`. Named accounts set
`XDG_DATA_HOME` only in the child. Both paths require this account's own key.
The CLI receives the selected key and fixed Go base URL through inline config,
so inherited provider environment keys cannot select another account.

Users sign in at the OpenCode console and paste the Go key in Settings, even
when no CLI is installed. Key presence is labelled **Go key saved**, not a
claim that the service has validated it. Keys are write-only through the
settings API, stored using private temporary files and rename (0600 on POSIX),
and never returned in account responses. Saving preserves other providers in
the shared auth file. `agenttik-mode` stores the execution preference beside it.
The optional terminal flow runs `opencode auth login --provider opencode-go`.

## Direct coding turns

Direct execution uses `POST https://opencode.ai/zen/go/v1/chat/completions`,
`User-Agent: agenttik/1.0`, and a stable `x-opencode-session` UUID per
conversation. Bearer authentication uses the selected Go key. The picker
exposes the documented Chat Completions models that work with either backend;
Messages/Responses-only Go models are not offered by this adapter.

SSE text, reasoning, fragmented function calls, and token accounting map to
agent events. A tool loop runs at most 64 steps. Plan permits project reads
and directory listings; Workspace adds file writes; Full adds detected shell and Python commands
with a 60-second timeout. The shared runtime loads project AGENTS.md, detects
available executables, and asks for completion, verification and a final summary
([077](077-direct-api-runtime.md)). Go's `os.Root` confines file operations, including
symlinks, to the project. Shell processes are killed as a group on timeout or
cancellation. Each file read/tool output is limited to 64 KiB, each write to
1 MiB, and HTTP/history data to 16 MiB. HTTP requests time out after five
minutes. Truncated/error streams fail the turn rather than report success.

Conversation history is saved privately under
`<home>/opencode/agenttik-sessions/direct-<uuid>.json`. Resume requires the
same account and work folder. Isolated title/outcome requests have no tools
and do not save history. There is no automatic context compaction; large
conversations require a new task. An unsuccessful turn saves the prompt and completed tool results, since tools
may already have changed files. Interrupted response fragments are not saved.

Direct mode has basic file and shell tools, without the CLI's plugins, MCP,
subagents or image understanding. File writes create missing parent
folders; Full shell/Python access can run tests. Subscription
allowance bars are absent because no supported usage-query endpoint is used.

## CLI turns

`opencode run --format json --model opencode-go/<model>` reads the prompt on
stdin. CLI sessions resume with `--session ses_…`. JSONL text, reasoning,
tool results and step usage map to the same events; only a completed turn
emits `done`. Permissions are supplied with `OPENCODE_PERMISSION`: read/list
in Plan, edits in Workspace, general tools in Full, no tools for isolated
requests. Questions are denied because the web UI cannot answer CLI prompts.
Automatic sharing and update checks are disabled for the child.

## Verification

Go unit tests cover account separation, key preservation and file modes,
CLI selection and event parsing, streamed direct tool calls, persistent resume,
headers, cancellation, malformed streams, and file/permission boundaries.
Server tests cover write-only keys and refusal of unsupported direct login.
The subscription browser suite covers Go login without a CLI and preference
persistence. Tests use fake processes and HTTP servers; no paid Go request
has been made and no live subscription account has been validated.
