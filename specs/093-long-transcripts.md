# Long transcript rendering

The main conversation groups consecutive tool calls into rows. Conversations
with at most 80 rows render in full. Longer conversations initially mount the
newest 12 bodies, then use one IntersectionObserver to mount bodies within
800 pixels of the scroll viewport. Other rows retain lightweight DOM shells,
not message components, parsed Markdown or action controls. Engines without
IntersectionObserver render all bodies.

Unseen rows reserve 120 pixels. When a body leaves the viewport its measured
height replaces that estimate. The native scrollbar therefore refines as
history is visited. Measurements use a WeakMap keyed by the open tab, with
row index, message id and width checks. Tab switches release all mounted
bodies; closing a tab releases its measurements. Source messages and row
shells still use memory proportional to history length.

Editors, open threads, expanded tool groups, selected text and focused controls
keep their row bodies mounted outside the viewport. Closing these controls or
clearing the selection lets the row unload. Human-message navigation and sticky
prompt expansion mount their target before scrolling. Following live output
preserves the bottom as nearby bodies change height; reading older messages
does not resume following automatically.

Page find temporarily mounts all transcript bodies to search rendered text,
including phrases across Markdown formatting. Closing it saves heights and
releases bodies outside the viewport again. Full-history search and deliberately
expanded content can therefore use more memory than ordinary reading. Open
threads render their own exchanges in full.

TranscriptRows owns the history shells and memoizes unchanged rows.
TranscriptRow owns each mounted body. The transcript clock is injected into
bodies that need it; its 250 ms tick does not update the history list.
Turn footers and output/thread indexes use linear work and retained memory.

## Verification

Build the UI and web binary, then run serially:

```
cd e2e
npx playwright test tests/transcript-performance.spec.js --workers=1 --repeat-each=3
```

The benchmark seeds a disposable database with 5,000 messages and 2,500
finished turns. It measures opening to a frame after the latest body mounts,
mounted bodies, DOM nodes and retained JavaScript heap after forced collection.
Heap is the increase over the project view, not process RSS. Normal-mode gates
require opening within two seconds, fewer than 80 bodies and under 50 MiB of
additional retained heap. Separate cases cover scrolling to both ends,
navigation, editing across scrolling and mobile resizing, full rendered-text
search, and expanded tools released after closing outside the viewport.

For a baseline binary, set `AGENTTIK_E2E_BIN` and `PERF_BASELINE=1`, and select
`--grep '5,000'`. Baseline mode waits until all bodies mount before measuring
memory. The settled sample includes an additional 500 ms wait in both modes;
it measures the deferred rendering backlog, not just first paint.

Three serial Chromium runs on the development Linux host, at 1440 × 900,
gave these medians against baseline `0650af8` (which already has linear turn
footers). These compare ordinary reading with page find closed:

| Measurement | Baseline | Current |
| --- | ---: | ---: |
| Latest body and next frame | 350 ms | 289 ms |
| Settled sample, including polling and 500 ms wait | 17,432 ms | 1,177 ms |
| Mounted message bodies | 5,000 | 7 |
| DOM nodes in the page | 515,072 | 30,779 |
| Additional retained JS heap | 637.67 MiB | 20.45 MiB |

The heap reduction is about 97%. These measurements cover Chromium rendering
and do not measure native WebKit performance or server memory. All 185 web unit
tests and the focused conversation browser regressions pass.
