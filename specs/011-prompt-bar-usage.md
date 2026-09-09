# Prompt-bar usage indicators

## Outcome

The model control is a fuzzy-search palette. It preserves provider groups and
favourite model-and-effort combinations, and searching matches both the display
name and model id.

The prompt bar shows context usage as a circular ring. Hovering it reveals the
context count and window plus the session totals.

When the selected provider reports subscription buckets, the prompt bar also
shows one thin bar per allowance window. Hovering reveals the current percent,
the exact window/reset time, plan, and any reached-limit status. Codex uses its
local app-server's read-only account/rateLimits/read method; no credentials
or estimated usage are stored by agenttik. Providers without that method show
no allowance bars.

## Validation

go build ./... and npm run build passed. No tests were run.
