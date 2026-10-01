# Tool approvals and questions

A task's permission mode ([003](003-providers.md#permission-modes)) settles
most tool calls before the turn starts. What it leaves open — a destructive
command, a connector tool whose policy says "needs approval" — is put to the
user mid-turn instead of being refused. The agent can also ask the user
questions mid-turn, with options and typed answers.

Claude Code and Codex ask. Copilot, OpenCode and the direct API runtime still
refuse whatever their mode does not allow and cannot ask questions.

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

Questions are the `AskUserQuestion` tool, asked the same way. Its input is
`{"questions":[{"question","header","options":[{"label","description"}],
"multiSelect"}]}`. Questions have no id, so the question text is the id.
Answering allows the call with `answers` added to the input, question text to
answer, several picks joined with `, `. Typing is always allowed, as the CLI's
own "Other" is. Skipping denies the call.

A `control_cancel_request` withdraws a request. The `result` line closes
stdin, which is what lets the CLI exit; without it a stream-json CLI waits for
another prompt. A result with background tasks running leaves stdin open
([003](003-providers.md#claude-code)). Any other control request subtype gets an error response, so
the CLI never waits on something agenttik cannot answer. Isolated requests
(titles, outcomes, commit messages) keep plain text stdin and are never asked
anything.

Measured against Claude Code 2.1.285: `--permission-prompts host` alone (the
default) is not enough; without `--permission-prompt-tool stdio` the CLI
denies the call without sending a request.

## Codex

app-server sends JSON-RPC requests to its client and waits for the result.
The read loop keeps going while one waits; the result is written whenever the
user answers. Measured against codex-cli 0.159.2:

| Request | Asked as | Result |
|---------|----------|--------|
| `item/commandExecution/requestApproval` | Shell: the command, its directory and reason | `{"decision":"accept"\|"decline"}` |
| `item/fileChange/requestApproval` | Edit files: the paths from the item's `item/started` | `{"decision":"accept"\|"decline"}` |
| `execCommandApproval`, `applyPatchApproval` (v1) | the same | `{"decision":"approved"\|"denied"}` |
| `item/tool/requestUserInput` | questions, by `id`; `isOther` allows typing, `isSecret` hides it | `{"answers":{id:{"answers":[...]}}}`; skipping sends `{}` |
| `mcpServer/elicitation/request`, form, no fields | "*server* MCP tool": the message and `_meta.tool_params` | `{"action":"accept","content":{}}` or `decline` |
| `mcpServer/elicitation/request`, form with fields | questions, one per field in the server's order | `accept` with typed `content`, or `decline` |
| `mcpServer/elicitation/request`, url | not asked | `decline` |
| anything else | not asked | JSON-RPC error `-32601` |

Codex asks before an MCP tool call with an empty-form elicitation tagged
`_meta.codex_approval_kind = "mcp_tool_call"`. Form fields map to questions: an
enum, titled `oneOf`/`anyOf` or boolean gives options (titles shown, values
sent; booleans as Yes / No), an array of those allows several picks, and a
string or number field takes typed text, sent as a number when it parses.

`serverRequest/resolved` withdraws a request. It also follows each answer;
the runner ignores a withdrawal for a request no longer pending. The question
tool exists outside Codex's plan mode only behind
`features.default_mode_request_user_input`, which app-server is started with.
Approvals reach the client only under the workspace policy
([003](003-providers.md#permission-modes)), with the reviewer forced to `user`.

## Provider-neutral shape

A provider emits `approval` with an `agent.Approval` and `approval_resolved`
when it withdraws one. The approval carries `id` (unique across turns),
`tool`, `description`, `input` as JSON, and, for questions, `questions`: each
with `id`, `header`, `question`, `options` (`label`, `description`), `multi`,
`other` (typing allowed) and `secret`. `Answer(Reply)` resumes the turn, where
`Reply` is `{allow, answers}` and `answers` maps a question id to the picked
labels or typed text.

`CheckReply` refuses an allowing reply that leaves a question unanswered,
gives a single-choice question two answers, gives a blank answer, or picks an
option that was not offered where typing is not allowed.

## Runner

Pending requests live in the runner's memory, keyed by id: they exist only
while their turn's process does, so nothing goes in the store. Each one is
published to the session topic and to the project topic, so the sidebar of
every window marks the task whether or not its tab is open.

`Answer` checks the reply first — a bad one is `ErrBadReply` (400) and the
request stays pending — then takes the request out of the map before
answering, so when several windows answer at once the first wins and the rest
get `ErrApprovalGone` (409). Every settlement publishes `approval_resolved` to
both topics, with `allowed` set when the user answered. When a turn ends —
finished, failed or stopped — whatever it left pending is withdrawn the same
way.

## Answering for the user

Settings › General ([017](017-general-settings.md)) decides what happens to a
request nobody answers, read when the request arrives:

- `wait` — it waits. A turn waiting on the user holds its project's queue the
  way any long turn does; Stop ends it.
- `timeout` (default, 30 s) — the runner starts a timer with the request and
  sends it with `expires_in_ms`. When the timer fires, agenttik answers through
  the same path as a click, so a click and the timer race safely: whichever
  takes the request first wins. The resolution carries `auto: true`.
- `immediate` — agenttik answers in the consume loop, before publishing; no
  window sees the request.

`AutoReply` is the answer: allow a tool call; for questions, the option whose
label contains "recommended" (case-insensitive), else the first. If any
question has no options there is nothing to guess and the request is
declined.

`POST /api/sessions/:id/approvals/:approval/hold` stops a countdown — the
user saying "I will answer this, wait for me". The request is published again
without `expires_in_ms` and with `held: true`, and every window swaps its
countdown for "Timer paused". It then waits as under `wait`. A request taken out of the map — answered, withdrawn or
dropped — has its timer stopped.

The remaining time travels as a duration, not a time: a remote window's clock
can differ from the server's. `GET /api/approvals` reports what is left as of
the call, and a window turns it into a local deadline on receipt.

## HTTP

| Method | Path | |
|--------|------|-|
| GET | `/api/approvals` | every pending request: `session_id`, `project_id`, `turn_id`, `created_at`, `approval` |
| POST | `/api/sessions/:id/approvals/:approval` | `{allow, answers?}` — 204; 400 if the answers are incomplete; 409 if already answered or withdrawn |
| POST | `/api/sessions/:id/approvals/:approval/hold` | stop its countdown — 204, or 409 if gone |

## UI

Each window reads `/api/approvals` whenever its stream opens — the first open,
every reopen after a tab change, and every reconnect — so a request published
while it was not listening is still drawn. Events keep it current after that.

- An approval card shows the tool, its description, the input (a shell
  command as itself, other JSON indented) and Allow / Deny.
- A question card shows each question with its header, options as radio or
  checkbox rows with their descriptions, and a text field where typing is
  allowed (a password field for secrets). In a single-choice question typing
  replaces the pick and picking clears the text. Answer stays disabled until
  every question has an answer; Skip declines. The rules are in
  `web/src/questions.js`.
- A counting-down request shows "Allowing in 23s" or "Answering for you in
  23s" beside its answer buttons, with **Pause timer**, which holds it for
  every window; a held one says "Timer paused · waiting for your answer". The
  countdown is `ApprovalTimer.vue`, shared by both cards.
- The working line says "Waiting for approval" or "Waiting for your answer".
- The task's sidebar and project-list rows show a shield ("Waiting for you"),
  and the project dot counts a waiting task as needing attention, so a folded
  project still says so.
- With task sounds on ([017](017-general-settings.md)), each request dings
  once.

A remote window ([064](064-remote-connections.md)) and a browser on the
exposed server ([043](043-exposed-server.md)) use the same routes, so they see
and answer the same requests.

Tests: `agent` (reply checking), `claudecode` and `codex`
(`appserver_requests_test.go`) for the protocols, `runner/approvals_test.go`
(fan-out, first answer wins, stop withdraws, incomplete answers, the three
modes and holding),
`server/api_approvals_test.go` (routes), `web/src/questions.test.js`, and
`e2e/tests/approvals.spec.js` (two windows, reload, stop, questions, the
setting, a timeout, holding from another window, answering at once). The fake
provider's `@approve <tool> <input>` asks for approval and replies `allowed`
or `denied`; `@ask <question> | <a>, <b>` asks a question and replies
`answer=<picks>` or `declined`.
