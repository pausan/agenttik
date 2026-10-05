# Large text files

Files with 10,000 lines or more, or more than 400,000 UTF-16 characters, use
one native scrolling textarea. The footer says `plain text · no wrap`.
Smaller files use the highlighted, wrapping overlay. Large files allocate no
highlighted HTML or line elements and do not lay out a second text copy.
Line counts scan newlines without splitting the file into strings.

Both editors support editing, Save / Ctrl+S, Tab indentation, undo/redo,
unsaved-tab protection and transcript line links. Large editors retain their
own horizontal and vertical scroll position across tab and mode switches.
Find searches unsaved textarea text and selects and reveals the current match;
it does not paint every match simultaneously in the native textarea.

The existing 2 MiB read/edit limit still applies. Truncated files stay read-only.
This is a bounded rendering path, not a streaming text editor. Undo history
still keeps text snapshots and browser editing still operates on the whole
loaded string.

`e2e/tests/large-files.spec.js` creates distinct 12,000- and 30,000-line JS
files and measures opening through the next frame and keydown-to-frame p95
for 30 real keystrokes. Run serially; `PERF_BASELINE=1` omits new behavior
assertions. Checks cover the absence of the overlay, typing below 50 ms p95,
opening below one second, undo/redo, find and saving. Navigation checks cover
linked lines and independent native scroll positions; read-only checks keep
Tab from changing truncated text.

Three Chromium runs on the development Linux machine gave these medians:

| Lines | Baseline open | Current open | Baseline typing p95 | Current typing p95 |
| --- | ---: | ---: | ---: | ---: |
| 12,000 | 367 ms | 103 ms | 123 ms | 20 ms |
| 30,000 | 270 ms | 174 ms | 118 ms | 34 ms |

Baseline is commit `eaa9253`; these measure browser interactions, not native
WebKit. The two comparison jobs overlapped for part of their runs, so the
numbers show the scale of improvement, not a controlled CPU-isolated result.
