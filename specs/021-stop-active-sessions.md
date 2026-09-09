# Stop active sessions

## Outcome

An active session and a session with queued prompts cannot be archived. Their
sidebar row shows **Stop** instead of the archive control. Stop cancels a
running turn and removes every queued prompt for that session, so the project
scheduler cannot start it later.

The server also rejects an archive request for either state. This keeps the
rule intact for callers outside the sidebar.

## Validation

`make ui` and `go build ./...` passed. Tests were not run, per the prototype workflow.
