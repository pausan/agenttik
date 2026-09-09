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

## UI unit tests

Only the pure logic is worth unit testing — the folder picker's fuzzy matcher,
where the ranking rules are easy to break and hard to eyeball. Everything else
is rendering, which the browser tests cover for real. Plain `node:test`, no
runner to install.

## Browser tests

Playwright, driving the built `bin/agenttik-web`. **Each test gets its own
server**: `e2e/fixtures.js` spawns one on port 0 with a fresh temp database and
tears it down afterwards. That is what lets them run in parallel and removes
every ordering dependency — no test can see a project another one added. The
whole suite is about six seconds.

The `page` fixture also fails a test on any error the page logs. Failed
requests are excluded: the browser logs one for every 4xx, and a test types a
path that cannot resolve on purpose, to check the picker shrugs it off.

Tests use the agenttik checkout itself as the project under test, so the tree,
the git status and the folder picker all read something real.

## What is not covered

A live turn. Running one spends a real subscription, so nothing in the suite
sends a prompt; the transcript's streaming path is exercised by hand. The
Codex provider is a stub, so only its command shape is unit tested.
