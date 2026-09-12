# Stopping an active task

A task that is running, or that holds queued prompts, cannot be archived. Its
sidebar row shows **Stop** in place of the archive control.

Stop cancels the running turn and removes every queued prompt for that task, so
the project scheduler cannot start it again later. It is also what returns a
task waiting out a provider outage to idle
([045](045-provider-outage-retry.md)).

Queued prompts move into the saved transcript as user messages, in queue
order, in the same database transaction that removes them from the queue.
They have no turn attached because they did not run. Their text survives
stopping from a closed tab, reopening the task, and restarting the app.
An open tab reloads the saved transcript after Stop. Existing draft text is
left untouched; saved prompts can be copied or edited from the transcript.

The server rejects an archive request for either state, so the rule holds for
callers outside the sidebar. The same rule stops a busy project being archived;
see [041](041-project-archiving.md).
