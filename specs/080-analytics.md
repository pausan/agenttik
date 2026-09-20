# Analytics

Status: implemented. Open **Analytics** in the command palette. The full-screen
page is loaded on demand and covers every retained project and task in the
current profile, including hidden and archived entries.

## Display

Rolling 1, 7, 14, 30, 90 and 365-day windows use turn start times. The default is
7 days. Provider grouping combines subscriptions; subscription / connection
grouping separates them. Projects and task/subscription rows rank by reported
USD cost, input tokens, output tokens or turn count. Shares use the selected
metric and the containing provider or subscription total. Expand a project for
its tasks; clicking a task opens it. Refresh updates the window and totals.

Summary cards show reported cost, input tokens, output tokens and turns. Cache
reads and writes remain separate because provider token accounting differs.
Recorded cost is not a subscription invoice: fixed fees, external usage and
unreported costs are excluded. A dash means no positive cost was recorded;
the data cannot distinguish a free turn from missing cost. Each group shows how
many turns recorded positive cost, making partial coverage visible. Running
turns count, but their token/cost figures arrive when their usage is persisted.
Deleted projects/tasks no longer contribute. Empty, loading and retry states
are explicit.

## Storage and API

`GET /api/analytics?days=7` accepts integer days from 1 through 3650 and returns
`from`, `to` (Unix milliseconds) and task/provider/account aggregate `rows`.
The half-open interval is `[from, to)`. One SQL query uses the turn start-time
index and groups before transferring data; transcripts are never loaded.
Existing profile routing and private-mode storage scope the endpoint.

Turns snapshot `provider` and `account_id` at creation, so later task model or
subscription switches do not move past usage. Migration copies each existing
task's current provider/account into its old turns and marks them with
`attribution_inferred = 1`; the page warns that earlier switches cannot be
reconstructed. New turns have `attribution_inferred = 0`.

Store tests cover window boundaries, provider/account switches, aggregation,
hidden/closed history and migration. UI unit tests cover grouping, ordering and
shares; browser tests cover command-palette access and page interactions.
