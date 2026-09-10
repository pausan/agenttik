# Retry Claude Code startup while it updates

## Outcome

Claude Code can replace its versioned executable after AgentTik has found it
on `PATH`, leaving a short window where `exec` answers `ENOENT`. The Claude
provider now recreates and starts its command once on that specific error.
All other startup errors still reach the user unchanged.

## Validation

Pending a compile check. No tests were added or run because this prototype's
current project rules exclude them unless requested.
