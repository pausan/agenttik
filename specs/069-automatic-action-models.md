# Automatic action models

Settings → Models → Automatic actions has separate model, subscription and effort
choices for merge/rebase conflict resolution and commit-message generation.
Choices are shared by windows in the same profile and persist in SQLite.
Use default removes an override; General → Reset defaults removes all overrides.

Conflict resolution defaults to GPT-6 Astra High on Codex, then Opus High on
Claude, then Astra/Opus on other available providers with a signed-in default subscription. If none is available, the
user can select another model. Clean Git operations do not need a model. Conflict requests allow workspace tools
without retaining a provider conversation; metadata-only requests keep tools disabled.
Commit messages use the first available provider's lightweight model, preferring
Codex (GPT-5.6 Luna, none), then Claude (Haiku, low). Explicit choices are never
silently replaced with another subscription. Task model choices are independent.
Commit generation uses only a bounded staged diff, in an isolated read-only
request outside the repository. Generating does not commit.

`GET /api/action-models` lists action IDs, labels, descriptions, effective choices
and whether each is custom. `PUT /api/action-models/:action` saves
`{provider, account_id, model, effort}`; `{}` restores the default. The server
validates models, efforts and subscriptions. The `action_models` table has one
row per overridden action. Future actions add a descriptor and execution policy
without a database migration or a separate settings component.
