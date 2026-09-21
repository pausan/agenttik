# Analytics

Status: implemented. Open **Analytics** in the command palette. The full-screen
page is loaded on demand and covers every retained project and task in the
current profile, including hidden and archived entries.

## Display

Rolling 1, 7, 14, 30, 90 and 365-day windows use turn start times. The default is
7 days. Provider grouping combines subscriptions; subscription / connection
grouping separates them. Model grouping combines all efforts within each provider/model. Projects and task/subscription rows rank by reported
USD cost, input tokens, output tokens turn count, cache reads or cache writes. Shares use the selected
metric and the containing provider, subscription or model total. Expand a project for
its tasks; clicking a task opens it. Refresh updates the window and totals.

The **Models & effort** tab shows vertical bars for recorded provider/model/effort
combinations, sorted left to right by the selected metric, highest first. Only
combinations used in the period appear, including those with zero reported cost.
Missing model/effort values are labeled explicitly. Both tabs share period,
ranking and refresh controls. Task rows combine model/effort slices.

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
`from`, `to` (Unix milliseconds) and task/provider/account/model/effort aggregate `rows`.
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

## Codex cost estimates

Completed Codex turns retain their selected model, effort and raw token counts,
plus `estimated_cost_usd` and `cost_estimate_basis`. Provider-reported `cost_usd`
remains separate and takes precedence. Analytics rows expose summed
`estimated_cost_usd` and `estimated_cost_turns` for future display; the current
page still displays reported cost only. Existing turns are not repriced.

The versioned rate table in `store/pricing.go` uses [OpenAI standard API
pricing](https://developers.openai.com/api/docs/pricing), verified 2026-09-21,
for Astra, Sol, Terra, Luna and GPT-5.3-Codex. Codex input totals include cache
reads and writes; subtract those before applying the ordinary input rate.
Output includes reasoning and is charged once. Unknown models, missing usage,
and invalid counters have no estimate (empty basis), rather than a guessed price.

These are short-context API-equivalent estimates, not subscription invoices.
They use the turn's selected model for all reported tokens, including children;
the current protocol tracker does not attribute child tokens to individual
models. Service tier, per-request long-context thresholds, regional uplifts,
and tool fees are not available for this estimate. Raw usage is retained so
future analytics can refine the calculation. Rates are stored indirectly by
the dated basis; changing the table requires a new basis version.
