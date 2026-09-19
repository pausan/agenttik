# UI interaction performance

Archived tasks remain in the backend until the project's Tasks pane opens.
Only one page plus a lookahead row is retained, and leaving the pane releases
it. Stats and Jobs also load on demand. New-task model defaults fetch one
newest task. See [030](030-project-tasks.md).

The file editor updates highlighted lines individually, preserving unchanged
DOM. The highlighted copy determines height and the transparent textarea
fills it. There are no synchronous textarea height measurements on input;
wrapping, empty trailing lines and viewport changes use normal CSS layout.
Highlighting still carries multiline lexer state between lines and escapes
all source text.

Transcript follow-scroll observes message-list replacement, length and the
last message's content, without recursively watching historical messages.
Long transcripts paint 40 rows first, then add batches of 40 between frames.
Tab switches cancel pending batches; saved reading positions restore all
rows. Live output preserves rows already visible.

Settings mounts panes on first visit, retaining them until the dialog closes.
Searching mounts all panes so result counts cover every section. Modal and
slideover transitions and menu entrance/exit animations are disabled. Profile
switching retains a full page navigation and its existing storage isolation.

## Reproducing measurements

Build the UI and web binary, then run:

```
cd e2e
npx playwright test tests/performance.spec.js --workers=1 --repeat-each=3
```

The fixture uses a disposable database with 5,000 archived tasks, one open task
and 200 messages. It measures startup, opening and closing the task, real
prompt/file keystrokes, Settings, and a profile switch. Archive assertions
check bounded initial history requests and pages, searching across history,
and restoring a task. The file is `web/src/store.js`, edited only in memory.

Typing is keydown-to-next-frame p95 over 30 real keystrokes. Task open uses the
browser clock through a frame after the prompt appears. Startup is navigation
to a visible task row; closing, Settings and profile measurements include
Playwright action/assertion overhead. These are Chromium measurements, not
native window/tray launch measurements. No live provider runs are involved.

`AGENTTIK_E2E_BIN` selects a separately built binary. `PERF_BASELINE=1` skips
new archive-behavior assertions when comparing an older binary. Optional
`PERF_PROFILE=/tmp/editor.cpuprofile` writes a Chromium CPU profile for typing.

Backend allocation benchmark:

```
go test ./app/internal/store -run '^$' -bench BenchmarkArchivedSessions -benchmem
```

On the development Linux / Ryzen AI 9 365 host, reading 5,000 archived rows
cost 57.8 ms and 12.3 MB allocated per request. Reading 26 rows cost 0.35 ms and
54 KB. Opening the project no longer transfers the 2.26 MB archive payload.
These are request allocations and decoded payload bytes, not process RSS.

Three serial Chromium runs against baseline commit `4ef5345` and the changed
build gave these medians (milliseconds, same machine and source-file fixture):

| Interaction | Baseline | Changed |
| --- | ---: | ---: |
| Open 200-message task | 270 | 133 |
| File typing p95 | 74 | 28 |
| Prompt typing p95 | 11 | 16 |
| Open Settings | 293 | 245 |
| Close Settings | 94 | 107 |
| Switch to empty Work profile | 261 | 229 |
| Close task | 98 | 84 |
| Restore project with large archive | 259 | 268 |

The larger typing and transcript gains are repeatable; small differences in
navigation and closing overlap run-to-run noise. Prompt typing is within a
60 Hz frame. Archive deferral reduces retained data and background work, but
these fixtures do not demonstrate a startup-time improvement. The separate
cold-start gate passed its 30-project / 300-task case in 434 ms.

Validation covers Go tests and vet, 109 web unit tests, the browser suite with
focused reruns of corrected cases, the cold-start budget, and the native
WebKit scrollbar-layer regression. The optional real Bekko download test is
skipped unless explicitly enabled. Editor browser checks cover wrapping,
trailing lines, resizing, undo/redo, find, file links and scroll restoration.
