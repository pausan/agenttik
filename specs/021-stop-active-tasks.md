# Stopping an active task

A task that is running, or that holds queued prompts, cannot be archived. Its
sidebar row shows **Stop** in place of the archive control.

Stop cancels the running turn and removes every queued prompt for that task, so
the project scheduler cannot start it again later. It is also what returns a
task waiting out a provider outage to idle
([045](045-provider-outage-retry.md)).

The words are not lost with the queue. If the task is open in a tab, its
removed prompts go back into that tab's draft — the same box an unsent prompt
waits in ([012](012-task-queue.md)) — in the order they would have run, ready
to edit and send again. Nothing is overwritten if something is already typed
there, the same rule a failed send or enqueue follows.

The server rejects an archive request for either state, so the rule holds for
callers outside the sidebar. The same rule stops a busy project being archived;
see [041](041-project-archiving.md).
