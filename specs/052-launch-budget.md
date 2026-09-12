# Launch budget

A restored window draws its sidebar, tab strip and task in front within one
second. The browser test measures from `page.reload()` to those visible pieces
of text, using `performance.now()` in the page so browser-launch overhead is
not counted.

At startup, projects and task rows are read together. Provider discovery and
schedules begin at the same time but do not delay that first screen: provider
probes may start a CLI and arrive later without making the rest of the window
wait.

Saved tabs fetch their fresh API data together, then join the strip in their
saved order. The order matters for a project's one context tab and for a file
that follows its owner; fetching order does not. The sidebar project and task
lists are read once for the whole restore, and the event stream opens once
after it.

Views reached only after a click or shortcut are separate browser chunks.
Dialogs mount on demand; the add-project dialog remains mounted after first
use so a minimized clone can continue. A long task initially draws its newest
40 transcript rows, then fills in the rest after that first paint. Static UI
and ordinary API responses use fast gzip compression. The event stream stays
uncompressed so each frame reaches the browser without a compression buffer.

`e2e/tests/startup.spec.js` restores three projects, three tasks and a file.
It checks the one-second budget and verifies that the sidebar lists are each
read once while each restored tab is fetched once.
