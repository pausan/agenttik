# Tool approvals

A task's permission mode ([003](003-providers.md#permission-modes)) settles
most tool calls before the turn starts. What it leaves open — a destructive
command under `workspace`, a connector tool whose policy says "needs approval"
— is put to the user mid-turn, the way the other Claude apps ask, instead of
being refused.

Only Claude Code asks. Codex, Copilot, OpenCode and the direct API runtime
still refuse whatever their mode does not allow.

## Claude Code

Task turns run with `--input-format stream-json --permission-prompt-tool
stdio`. The prompt goes in as one `user` JSON line and stdin stays open. A
tool call the mode does not settle arrives on stdout as:

```
{"type":"control_request","request_id":"<uuid>","request":{"subtype":"can_use_tool",
 "tool_name":"Bash","display_name":"Bash","description":"...","input":{...}}}
```

and the CLI waits. The answer is written to stdin:

```
{"type":"control_response","response":{"subtype":"success","request_id":"<uuid>",
 "response":{"behavior":"allow","updatedInput":<input unchanged>}}}
{"type":"control_response","response":{"subtype":"success","request_id":"<uuid>",
 "response":{"behavior":"deny","message":"The user denied this tool call."}}}
```

A `control_cancel_request` withdraws a request. The `result` line closes
stdin, which is what lets the CLI exit; without it a stream-json CLI waits for
another prompt.

Two requests are answered without asking. `AskUserQuestion` is denied with a
note to ask in the reply instead, because its answer is a choice, not a yes or
no. Any other control request subtype gets an error response, so the CLI never
waits on something agenttik cannot answer.

Isolated requests (titles, outcomes, commit messages) keep plain text stdin
and are never asked anything.

Measured against Claude Code 2.1.285: `--permission-prompts host` alone (the
default) is not enough; without `--permission-prompt-tool stdio` the CLI
denies the call without sending a request.

## Provider-neutral shape

A provider emits `approval` with an `agent.Approval` (`id`, `tool`,
`description`, `input` as JSON) whose `Answer(allow)` resumes the turn, and
`approval_resolved` when it withdraws one. The id is unique across turns.

## Runner

Pending approvals live in the runner's memory, keyed by id: they exist only
while their turn's process does, so nothing goes in the store. Each one is
published to the session topic and to the project topic, so the sidebar of
every window marks the task whether or not its tab is open.

`Answer` takes the approval out of the map before answering, so when several
windows click at once the first wins and the rest get `ErrApprovalGone`
(409). Every settlement publishes `approval_resolved` to both topics, with
`allowed` set when the user answered. When a turn ends — finished, failed or
stopped — whatever it left pending is withdrawn the same way.

There is no timeout. A turn waiting on approval holds its project's queue the
way any long turn does; Stop ends it.

## HTTP

| Method | Path | |
|--------|------|-|
| GET | `/api/approvals` | every pending approval: `session_id`, `project_id`, `turn_id`, `created_at`, `approval` |
| POST | `/api/sessions/:id/approvals/:approval` | `{allow: bool}` — 204, or 409 if already answered or withdrawn |

## UI

Each window reads `/api/approvals` whenever its stream opens — the first open,
every reopen after a tab change, and every reconnect — so a request published
while it was not listening is still drawn. Events keep it current after that.

- The transcript draws a card per request above the working line: the tool,
  its description, the input (a shell command as itself, other JSON indented),
  and Allow / Deny. The working line says "Waiting for approval".
- The task's sidebar and project-list rows show a shield, and the project dot
  counts a waiting task as needing attention, so a folded project still says
  so.
- With task sounds on ([017](017-general-settings.md)), each request dings
  once.

A remote window ([064](064-remote-connections.md)) and a browser on the
exposed server ([043](043-exposed-server.md)) use the same routes, so they see
and answer the same requests.

Tests: `claudecode` (control protocol), `runner/approvals_test.go` (fan-out,
first answer wins, stop withdraws), `server/api_approvals_test.go` (routes),
`e2e/tests/approvals.spec.js` (two windows, reload, stop). The fake provider's
`@approve <tool> <input>` directive asks and replies `allowed` or `denied`.
