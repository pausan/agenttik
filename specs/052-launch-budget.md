# Launch budget

The UI must render within **1,000 ms** of browser navigation: every active
project and its expanded open task rows in the sidebar, the restored tab
strip, and the main project or task view. A task view includes its transcript
and prompt; a project view includes its open task list. This applies to warm
reloads and cold browser caches. Browser process startup, native window
creation and server startup are outside this browser budget.

Projects and task rows load first. Saved tabs fetch their required data in
parallel and join the strip in saved order. A project requires only its
metadata and open tasks; statistics, daily metrics, archived tasks and
schedules arrive independently afterward. Statistics remain pending until
an actual answer arrives. Providers and the global schedule list start after
restoration, keeping CLI probes out of the initial request queue.

Restored file tabs appear without waiting for file contents or Git diffs.
They keep their owner, order and saved selection. Their body shows a local
loading message until the read completes; a failed read shows the error in
that tab. Ordinary file opens still wait for a successful read. Missing
projects and tasks are skipped during restoration. The sidebar lists are
read once and the event stream opens once after restoration.

Views reached only after a click or shortcut are separate browser chunks.
Dialogs mount on demand; the add-project dialog remains mounted after first
use so a minimized clone can continue. A long task initially draws its newest
40 transcript rows, then fills in the rest after first paint. Static UI and
ordinary API responses use fast gzip; event streams stay uncompressed.

`make test-startup` runs `e2e/tests/startup.spec.js` against the built web
binary, with one browser worker and no retries. It checks:

- A restored three-project workspace, three tasks and a file, including
  request counts and the task transcript in front.
- Cold task and project views with file contents, history, schedules,
  statistics and providers held until after the budget assertion. Releasing
  them must populate the deferred file and statistics views.
- A cold workspace with 30 projects and 300 open tasks, all sidebar task
  groups expanded, and 30 saved project views.

The page checks rendered text on animation frames and waits another frame
before reading `performance.now()`. Every timed case fails at 1,000 ms;
missing content fails after five seconds. The CI startup-budget job runs on
pushes and pull requests, and releases depend on it passing. These fixtures
are the enforced workload, not a guarantee for arbitrary hardware, database
sizes or remote network latency.
