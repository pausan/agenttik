# Smart Search

Settings → General → Project task search offers Fuzzy Search (default) and
Smart Search. The preference is local to each browser and survives reloads.
Only the project's open Tasks list uses this choice. Archived tasks use
backend title search and pagination ([030](030-project-tasks.md)); other
searches keep their existing matching.

The Go backend runs the MIT-licensed `hotchpotch/bekko-embedding-v1-a8m`
model at revision `c721113d59a1d91b447450324f51c4b3332c924a`. Native ONNX
Runtime 1.24.1 and the Hugging Face tokenizer library v0.1.4 load through
Go bindings without cgo or a Node.js requirement. Runtime libraries download
on first enable, with checksum verification by the runtime bindings. ONNX
Runtime uses the host's user cache; the tokenizer library, model and task
vectors live under `smart-search/` in Agenttik's data directory. The browser
ships no inference runtime, worker, model, or WASM asset. Task text stays on
the Agenttik host; no task text is sent to the model download service.

One service per backend shares a model and index across browsers. Loading,
indexing and queries are serialized. Nothing downloads or initializes at app
startup. Enabling starts indexing; disabling stops that browser's polling and
ignores pending results. Shared backend work continues for other clients and
keeps its model loaded until shutdown. Model downloads honor server shutdown;
native-library setup finishes within the bindings' own download timeouts.

The API is:

- `POST /api/smart-search/index`: starts or joins an indexing pass, returns
  202 with progress. The backend reads all tasks directly from its database.
- `GET /api/smart-search`: phase, percentage, completed/total, estimated seconds,
  error, and index version. Enabled browsers poll every 500 ms during preparation.
- `POST /api/smart-search/query`: `{query, ids}` → task-id-to-cosine-score map.
  Queries accept up to 16384 bytes and 100000 candidate IDs. Missing IDs are omitted.

Preparation shows runtime setup, model download percentage, then indexing
progress with counts and estimated seconds remaining. Failures show Retry in
Settings and project views. Project metadata changes and a 30-second browser
timer request refreshes. Concurrent refreshes coalesce. Index versions change
only when indexed content or membership changes.

Indexed text is title (or “Untitled task”), opening prompt preview, and archive
outcome, separated by newlines. Full transcripts are excluded. Inputs truncate
to 512 tokens. Mean pooling and L2 normalization produce 384-dimensional float32
vectors. Queries use the same model and reuse the latest query vector.
Revision-specific vector files match exact text; unchanged tasks reuse vectors
across refreshes and restarts. Invalid cache entries rebuild. Writes use temporary
files and rename. Deleted tasks are removed from memory and disk during refresh.
Browser IndexedDB caches from older builds are unused and can be cleared.

Queries debounce by 250 ms. Outdated responses cannot replace newer queries.
The frontend restricts results to the current project's tasks, filters scores
below 0.30 and sorts descending across the open tasks. The threshold
is a heuristic, not a probability. Empty queries keep open-then-archived order.
Selecting Fuzzy Search immediately restores title matching.

The task time filter applies in both modes: Last 24h, Last week (7 days), Last
month (30 days), All times (default). It uses `last_active_at`, inclusive at the
cutoff, and resets on project changes. Time filtering disables drag reordering
and resets pagination, just like a text filter.

Verification: Go tests cover cache reuse/restart, updates/deletion, query reuse,
coalescing, errors/retry, shutdown, download integrity and API validation. Browser
unit tests cover requests and stale-response cancellation. Playwright covers time
filters and disabling. Set `AGENTTIK_TEST_BEKKO=1` for the real Go model test and
`e2e/tests/smart-search.spec.js` retrieval test. Optional
`AGENTTIK_TEST_BEKKO_DIR` reuses Go integration-test downloads across runs.
