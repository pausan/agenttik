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
tears it down afterwards, with `AGENTTIK_FAKE_PROVIDER=1` set (see below) and a
`PATH` stripped of whatever directory holds a real `claude`, `codex` or
`copilot` on the machine running the suite. That is what lets tests run in
parallel with no ordering dependency and no dependence on what is installed
where: no test can see a project another one added, and none can end up
talking to a real CLI or spending a real subscription, whichever of the three
the host happens to have.

The `page` fixture also fails a test on any error the page logs. Failed
requests are excluded: the browser logs one for every 4xx, and a test types a
path that cannot resolve on purpose, to check the picker shrugs it off.

Tests use the agenttik checkout itself as the project under test, so the tree,
the git status and the folder picker all read something real.

Four files, one per area: `folder-picker` (the fuzzy walk over real
directories), `inspector` (which panes the right-hand strip offers a project
versus a task, and that the one in use survives switching), `projects` (add,
rename, delete, archive, the per-project task list and its filter) and `tasks`
(creating one, the model and effort pickers, sending a prompt, and everything
below that a live turn touches — see the fake provider below).

## The fake provider

`app/internal/agent/fake` is a fourth `Provider` that spawns no process and
holds no account: a turn is driven entirely by reading directive lines out of
the prompt, so a browser test can make an agent stream a reply, call a tool,
error out, or hang until stopped — without a CLI on PATH or a subscription to
spend. It answers `Available` unconditionally, which is what makes a test's
first "New task" click work regardless of what is installed on the machine
running the suite.

It is registered only when `AGENTTIK_FAKE_PROVIDER` is set — `fake.Enabled()`
is the one place that reads it — which `e2e/fixtures.js` does and nothing else
does. A normal `make build` or `make run` never carries it.

A prompt line starting with `@` is a directive; anything else is the reply,
streamed back word by word:

| Directive | Effect |
|-----------|--------|
| `@wait <ms>` | pause, cut short the moment Stop cancels the turn |
| `@tool <name> <input...>` | a tool call and its canned result |
| `@error <message>` | end the turn in error; a message containing an outage phrase ("rate limit", "connection refused", ...) exercises the retry queue exactly as a real provider's outage would — see [045](045-provider-outage-retry.md) |

An isolated request — title refinement, [020](020-task-titles.md) — is
answered differently: instead of reading directives, it replies "Refined: "
plus the subject line of the request it was actually asked to name, so two
tasks started from different prompts keep two different refined titles rather
than converging on one constant. `SubscriptionLimits` answers a fixed,
made-up allowance, so the prompt bar's bars have something deterministic to
show without needing a provider that answers a real one.

## What is not covered

A live turn against a real provider. Running one spends an actual subscription
and needs the CLI installed, so nothing in the suite does that; the fake
provider stands in for a turn's mechanics — streaming, tool calls, errors,
retries, queueing, titles, allowance bars — and a real CLI's own event mapping
is exercised by hand.
