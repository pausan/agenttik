# API providers and model availability

Settings → Providers separates Subscriptions from API Providers. API connections
are optional and disabled until a key is saved and the provider returns a usable
model catalog. API billing is separate from subscriptions. API providers require
no installed agent CLI and appear in all shared model pickers, including tasks,
queued prompts, schedules and automatic-action overrides.

## Connections

| Provider ID | Service | Protocol |
|---|---|---|
| `api-openrouter` | OpenRouter | Chat Completions |
| `api-openai` | OpenAI | Chat Completions |
| `api-anthropic` | Anthropic / Claude | Messages |
| `api-google` | Google Gemini | OpenAI compatibility |
| `api-groq` | Groq | OpenAI compatibility |
| `api-deepseek` | DeepSeek | OpenAI compatibility |
| `api-mistral` | Mistral | Chat Completions |
| `api-xai` | xAI | Chat Completions |

The instance supports any number of named connections per API provider.
A fuzzy-search provider dropdown, name and API key form adds a connection after
validating its catalog. Each has a stable generated ID (`api-<service>-<random-id>`),
its own credential and model cache, and a name shown alongside the service in model pickers.
Existing connections retain their IDs and credentials. Enable saves a key after fetching models;
Save key replaces it after the same check. An empty key keeps the saved key.
Refresh models fetches the catalog again. Disable retains the key but removes
models from settings and pickers; Delete removes the connection from settings
and pickers and clears its credential and model cache. Conversation history is retained. Failed validation preserves the last working connection.

Catalogs are cached on disk and read without network calls at startup or during
normal provider listing. Enabling and refreshing have a 20-second deadline.
OpenRouter validates `/key` before reading `/models/user`, respects account
catalog preferences, and only offers models advertising tools and text output.
Mistral uses its chat/function capabilities; other providers use conversational
family and endpoint exclusions because their lists do not expose tool support.
Anthropic pagination is followed. Discovery confirms catalog access, not billing
credit or a successful inference call. Retired/incompatible models can still fail
at inference; refresh the catalog or choose another model.

## Credentials and API

`GET /api/providers` includes `kind` (`subscription` or `api`) and, for API
connections, `api.enabled`, `api.key_set`, `api.key_url`, service `api.provider` and `api.label`.
`api.catalog` identifies service templates used by the add dropdown; unconfigured
templates are not shown as saved connections. It never includes keys.
`POST /api/providers/:provider/api` accepts `{name, key}` and adds a separate
connection, shared immediately with all profiles.
`PUT /api/providers/:provider/api` accepts `{key, enabled, refresh}` and returns
updated providers. `DELETE` at the same route removes the key. Existing server
and remote authentication protect these routes like the other settings routes.

Keys and cached models live in `<data-dir>/api-providers/<id>/connection.json`,
written atomically with mode 0600 on POSIX and private parent directories.
They are local credential files, not encrypted vault storage. Keys are neither
browser preferences nor SQLite fields. Requests use fixed HTTPS service URLs;
redirects are refused, and raw upstream error bodies are not exposed.
Added profiles share live connection state while keeping conversation files in
their own directory. Connection changes apply to every profile. Private mode
uses its temporary data directory. Legacy profile keys fill an empty shared
connection in profile order; an existing instance key takes precedence. Original
connection files are preserved under `api-providers/<id>/legacy/<profile-id>.json`
for recovery, then removed from the profile directory to prevent reimport.

## Models and tasks

Models contains available, signed-in subscription accounts and enabled API
connections only. Signed-out accounts, unavailable CLIs, disabled APIs and their
favourites are absent. Existing tasks keep their saved selection and history;
disabling prevents subsequent turns. Group/model visibility and favourites remain
saved so reconnecting restores preferences. New tasks skip unavailable recent
choices and use a signed-in account when the preferred subscription is signed out.

API rows and selected buttons name the connection and service (for example `Work · OpenAI API · …`).
Switching provider resets the provider conversation through the existing session
mechanism. API tasks use the [shared runtime](077-direct-api-runtime.md), keep
history under the provider's `sessions/` folder, and use provider-default reasoning
settings. No automatic small-model policy is supplied for paid APIs; automatic
operations can use explicitly selected API models. Task summaries are requested
in the runtime system prompt. There are no subscription allowance bars for APIs.

## Verification and limits

Go fixtures exercise discovery, filtering, invalid keys, private persistence,
shared connections, local model preferences, disable/removal, real session dispatch and token recording.
Browser tests exercise the settings tree, mobile layout, key forms, error display,
model visibility and task selection with mocked connections. Tests make no paid
requests. Live credentials and model availability have not been verified.

The runtime has bounded steps, execution time and output, but no automatic context
compaction, image understanding or local currency budget. Provider-side spending
limits should be set in the provider console. The task prompt has a **Tool access** selector: Read only, Edit project files
(default), or Full access for shell/Python commands. Changes are saved through
`PATCH /api/sessions/:id` with `permission` and refused during an active turn.
API conversation history is retained when permission changes. New tasks remember
the last task choice, and schedules inherit the selected permission. Workspace
confines edits to project files.

Implementation references: [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling),
[Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming),
[Gemini compatibility](https://ai.google.dev/gemini-api/docs/openai),
[OpenRouter catalog](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties),
[OpenRouter reasoning](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens),
[Groq compatibility](https://console.groq.com/docs/openai),
[DeepSeek API](https://api-docs.deepseek.com/),
[Mistral models](https://docs.mistral.ai/api/endpoint/models), and
[xAI API](https://docs.x.ai/developers/rest-api-reference/inference).
