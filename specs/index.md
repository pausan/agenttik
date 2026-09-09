# Specs index

| Doc | What it covers |
|-----|----------------|
| [001-architecture.md](001-architecture.md) | Process model, packages, request flow |
| [002-data-model.md](002-data-model.md) | SQLite schema, metrics, storage location |
| [003-providers.md](003-providers.md) | Provider interface, CLI invocation, event mapping |
| [004-ui.md](004-ui.md) | Layout, panels, HTTP API |
| [005-testing.md](005-testing.md) | The three test layers and how they are isolated |

Status: scaffold, verified end to end. A Claude Code session started from the
UI streams into the transcript and records per-turn token metrics; resume across
turns works. Several sessions can be open at once, each streaming into its own
tab. The Codex provider is a stub.
