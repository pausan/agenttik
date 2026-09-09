# Session queue

## Outcome

A prompt can be queued with the Send menu or Alt+Q. Queued prompts are persisted and marked with a clock in session lists.

Each project runs queued prompts only after its active turns finish. The scheduler keeps draining the session that just ran while it has queued prompts; otherwise it scans the projects open sessions from top to bottom.

## Validation

`make ui` and `go vet ./...` passed. No tests were added or run.
