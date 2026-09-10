# Retry Claude Code startup while it updates

Claude Code can replace its versioned executable after agenttik has found it
on `PATH`, leaving a short window where `exec` answers `ENOENT`. The Claude
provider now recreates and starts its command once on that specific error.
All other startup errors still reach the user unchanged.
