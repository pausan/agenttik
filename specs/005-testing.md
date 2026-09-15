# Testing

Tests cover logic, browser flows and the native desktop transport.

| Layer | Where | Command |
|-------|-------|---------|
| Go unit tests | `app/internal/**/*_test.go` | `make test` |
| UI unit tests | `web/src/*.test.js` (node:test) | `make test` |
| Startup budget | `e2e/tests/startup.spec.js` | `make test-startup` |
| Browser tests | `e2e/tests/*.spec.js` (Playwright) | `make e2e` |
| Linux desktop image uploads | `app/cmd/agenttik/desktop_images_linux_test.go` | `make test-desktop-images` |
| Linux desktop scrollbar layers | `app/cmd/agenttik/desktop_scrollbars_linux_test.go` | `make test-desktop-scrollbars` |

`make test` is the fast loop and runs the first two. `make e2e` builds the
server and drives a real browser against it; it is the slower one, kept
separate so the fast loop stays fast. `make test-startup` requires the built
web binary, installed e2e dependencies and Chromium; CI runs it separately
with one worker and gates releases on the one-second requirement
([052](052-launch-budget.md)).

`make test-desktop-images` needs the desktop build libraries and Xvfb. It runs
the actual image upload module inside Wails/WebKit in a subprocess, checking
that Blob and File images arrive intact without a native crash. It stays
separate from the fast loop because it requires a native display stack.

`make test-desktop-scrollbars` additionally needs xdotool, ImageMagick, and a
built UI (`cd web && npm run build`). It checks actual Wails/WebKit screenshot
pixels where opaque popups cover active horizontal and vertical scrollbars,
in light and dark themes and above a nested dialog. DOM hit testing alone
cannot detect the native scrollbar paint-order bug.

Tests are written with the change they guard, where writing one is reasonable
— see rule 6 in `AGENTS.md`. What exists guards the logic that is hard to
eyeball and the flows that are tedious to click through by hand.

Project-topic tests wait for the final `done` event with stats, allowing start
and title events before it. Reordering tests compare another project's session
position before and after the drag. Browser prompt-menu actions are scoped to
their dialog because the primary submit button can have the same label.

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

Tests are grouped by area: `folder-picker` (the fuzzy walk over real
directories), `inspector` (which panes the right-hand strip offers a project
versus a task, and that the one in use survives switching), `projects` (add,
rename, delete, archive, the per-project task list, its filter, and the outcome
an archived row grows — [051](051-task-outcomes.md)),
`subscriptions` (adding one, choosing which new tasks use it, the alias in the
picker, the subscription a turn really ran on, and what removing one says —
[050](050-subscription-accounts.md)), `tasks`
(creating one, the model and effort pickers, sending a prompt, and everything
below that a live turn touches — see the fake provider below) and `schedules`
(scheduling a prompt, and the archived job waiting on the project page until
it is restored — [028](028-scheduled-jobs.md)).

`mobile` covers touch navigation, task creation and sending, per-project
drafts, drawers and focus, narrow prompt controls and popovers, and preserving
the desktop panels across viewport changes — [055](055-mobile-layout.md).

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
| `@account` | reply with the login directory the runner resolved for this turn, `system` for the machine's own — the only way a browser test can see which subscription actually ran it ([050](050-subscription-accounts.md)) |

An isolated request is answered differently: instead of reading directives, it
echoes the subject it was handed. Which of the two it is asked is read off the
tag the runner wrapped that subject in, so the fake is never told: a title
request ([020](020-task-titles.md)) is answered "Refined: " plus the first line
of the prompt to name, and an outcome summary ([051](051-task-outcomes.md))
"Outcome: " plus the first line of the reply to describe. Two tasks started
from different prompts, or ending on different replies, keep two different
answers rather than converging on one constant.

`SubscriptionLimits` answers a fixed, made-up allowance, so the prompt bar's
bars have something deterministic to show without needing a provider that
answers a real one — and a different
figure for a named subscription than for the machine's own login, so a test
can see that swapping subscription re-read the allowance rather than
relabelling one.

It also implements `MultiAccount`, holding subscriptions the way the real CLIs
do, which is what lets `subscriptions.spec.js` exercise adding, choosing,
swapping and removing one with no CLI on PATH.

## What is not covered

The queue test uses a one-second fake turn and can miss the brief Queued
label under parallel load. The orchestrator tests hold work until explicitly
stopped when checking a waiting task. Phone tests emulate touch and viewport
changes in Chromium; real software keyboards still need device testing.

A live turn against a real provider. Running one spends an actual subscription
and needs the CLI installed, so nothing in the suite does that; the fake
provider stands in for a turn's mechanics — streaming, tool calls, errors,
retries, queueing, titles, outcome summaries, allowance bars — and a real CLI's
own event mapping is exercised by hand.
