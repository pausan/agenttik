# Smart Search

Settings → General → Project task search offers Fuzzy Search (default) and
Smart Search. The choice is local to this browser/device and survives reloads.
Only the Tasks tab of a project uses Smart Search. Settings, the launcher,
folder pickers and every other existing search keep their fuzzy matching.

Enabling downloads the MIT-licensed `hotchpotch/bekko-embedding-v1-a8m`
model at revision `c721113d59a1d91b447450324f51c4b3332c924a`. Its compact
`onnx/model.onnx` is selected with `dtype: fp32`: the export already quantizes
the vocabulary table while preserving the transformer weights. Transformers.js
4.2 runs it in a dedicated worker with single-thread WASM. The runtime is bundled
with the app and loaded only when enabled. No task text goes to a model service.

The model download has a percentage progress bar. It is followed by task
indexing, with a separate bar, completed/total count, and seconds remaining
estimated from elapsed time per completed task. Before the first task completes,
the UI says it is estimating. Both Settings and the project page show progress,
errors and Retry. Disabling terminates the worker and cancels pending work;
the downloaded files and completed embeddings remain cached for reuse.

Model files use the browser Cache API where available. Embeddings are stored
in IndexedDB, keyed by task id and exact indexed text, in a revision-specific
database. Browser storage can be cleared or evicted; a later enable rebuilds
what is missing. Each browser has its own cache and preference.

The initial index covers all tasks, including archived tasks, through
`GET /api/sessions?window=all&include_done=true&limit=0`. Indexed text consists
of title, opening prompt (the API's preview), and archive outcome. Full transcripts
are not indexed. Inputs are truncated to 512 tokens to bound CPU work. Mean
pooling and L2 normalization produce 384-dimensional float32 vectors; no prefixes are
added. Unchanged text reuses its stored vector. Project metadata changes trigger
a refresh, and a 30-second refresh catches changes elsewhere. Deleted tasks are
removed from the index.

Queries are debounced by 250 ms and embedded with the same pipeline. Inference
is serialized; outdated responses cannot replace newer queries. The latest
query vector is reused until its text changes. Results are restricted to the
current project's tasks, filtered to cosine similarity >= 0.30, and sorted
descending across both open and archived tasks. This threshold is a starting
heuristic, not a probability of relevance. An empty query keeps the ordinary
open-then-archived ordering. Preparing or failed Smart Search is explicit in the
UI; selecting Fuzzy Search immediately restores title matching.

The project task time filter applies in both modes: Last 24h, Last week
(7 days), Last month (30 days), All times (default). It uses `last_active_at`,
inclusive at the cutoff, and resets on project changes. Time filtering disables
drag reordering and resets pagination, just like a text filter.

Verification: task-search unit tests cover ranking, thresholds, fuzzy behavior,
time boundaries and ETA. Browser tests cover time filtering and cancellation.
Run `AGENTTIK_TEST_BEKKO=1 npx playwright test tests/smart-search.spec.js`
from `e2e/` for the real model download, cross-language retrieval, cached offline
reload, and indexing newly created/deleted tasks.
