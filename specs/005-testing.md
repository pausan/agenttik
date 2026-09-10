# Testing

Three layers, each cheap enough to run often.

| Layer | Where | Command |
|-------|-------|---------|
| Go unit tests | `app/internal/**/*_test.go` | `make test` |
| UI unit tests | `web/src/*.test.js` (node:test) | `make test` |
| Browser tests | `e2e/tests/*.spec.js` (Playwright) | `make e2e` |

`make test` is the fast loop and runs the first two. `make e2e` builds the
server and drives a real browser against it; it is the slower one, kept
separate so the fast loop stays fast.

This is a prototype, so tests are written when asked for rather than with every
change — see rule 6 in `AGENTS.md`. What exists is what guards the logic that
is hard to eyeball.

## UI unit tests

Only pure logic is worth unit testing here, and there are four pieces of it:

- `fuzzy.js` — the folder picker's matcher, where the ranking rules are easy to
  break and hard to eyeball.
- `tree.js` — building the sidebar Tree from a flat listing, and filtering it.
  The per-node highlight offsets in particular are invisible to the eye and
  trivial to get wrong.
- `markdown.js` — the transcript's renderer. It is small and hand-written, so
  its tests are what stand in for a library's own suite; they cover the block
  and inline forms and, as much as anything, that source HTML is escaped and
  unsafe link schemes are dropped.
- `api.js` — the shared timestamp formatters, which have to read the same in
  any locale.

Everything else is rendering, which the browser tests cover for real. Plain
`node:test`, no runner to install.

## Browser tests

Playwright, driving the built `bin/agenttik-web`. **Each test gets its own
server**: `e2e/fixtures.js` spawns one on port 0 with a fresh temp database and
tears it down afterwards. That is what lets them run in parallel and removes
every ordering dependency — no test can see a project another one added.

The `page` fixture also fails a test on any error the page logs. Failed
requests are excluded: the browser logs one for every 4xx, and a test types a
path that cannot resolve on purpose, to check the picker shrugs it off.

Tests use the agenttik checkout itself as the project under test, so the tree,
the git status and the folder picker all read something real.

The suite has not been carried through the sessions-to-tasks rename
([038](038-task-naming.md)) and most of it fails at `HEAD`: it still asks for a
`New session` button, an `Untitled session` row and a `Sessions` sidebar pane,
none of which the UI has. It needs rewriting against the current labels before
it is worth running again.

## What is not covered

A live turn. Running one spends a real subscription, so nothing in the suite
sends a prompt; the transcript's streaming path is exercised by hand.
