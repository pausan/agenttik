# Review notes

Things found while bringing these specs in line with the code, that a human
should settle. Nothing here has been acted on: the specs now describe what the
code does, and this is the list of places where that was not obviously the
intent.

## The sidebar's Sessions pane left machinery behind

The left strip is `Projects | Tree`. A third pane, Sessions — an archive list
with a text filter and a "how far back" window picker — used to sit between
them, and the specs still described it until this pass. It is gone from
`SideBar.vue`, but what fed it is not:

- `S.schedules` is filled by `refreshSchedules()` on every turn that starts or
  ends, and read by no component.
- `refreshSessions()` fills `S.sessions`, which now only drives `syncSessionTabs`.
- `TASK_WINDOWS` and `setWindow` are exported and called from nowhere, so
  `S.window` is stuck at its default `1h` and silently narrows both fetches
  above. Nothing in the UI can widen it.
- `SideBar.vue` still carries an `item.value === 'sessions'` branch in its tab
  template. It is unreachable — `tabs` holds only `projects` and `tree` — and it
  renders the label `Sasks`, a leftover of the sessions-to-tasks rename.

Either the pane is coming back or this is dead weight on every turn. Worth
deciding which.

## An archived schedule cannot be reached

[028](028-scheduled-jobs.md) says a paused schedule can be archived and later
unarchived. Archiving still works, but nothing draws it afterwards:
`ListProjects` attaches schedules with `ExcludeDone: true`, so the sidebar never
sees it, and the project page's Tasks tab reads `/api/sessions` only. The
Sessions pane was the one place archived schedules were listed.

`GET /api/schedules?include_done=true` still answers, so the row is intact and
only the way back to it is missing. The spec now stops short of promising a
restore path.

## Vocabulary in the shortcut registry

`shortcuts.js` groups `tab.close` and `tab.reopen` under `Conversations` while
every other task-related entry is grouped under `Tasks`. Settings draws those
group names, so the dialog shows both words for the same thing.

## Known failures, unchanged

- `TestDoneReachesProjectTopic` (`runner`) and
  `TestReorderSessionsDrivesProjectOrder` (`store`) fail at `HEAD` and have for
  several commits.
- `tasks.spec.js`'s "a second prompt queues behind a running turn and runs in
  order" fails at `HEAD` (cb5f2a0): `getByRole('button', { name: /Enqueue/ })`
  now matches two elements, the primary submit button (whenever Enter is bound
  to enqueue — the default, [015](015-settings-shortcuts.md)) and the Send
  menu's own Enqueue entry, which the same commit made always render
  regardless of which action is already primary. Reproduced against an
  unmodified checkout, so it is not one this pass caused. The test needs a
  locator scoped to the menu, e.g. the `dialog` role it already opens.
- [036](036-copilot-model-list.md) documents a live gap: a Copilot model that
  takes no reasoning effort is still offered the provider-wide levels, because
  `agent.Model.Efforts` is `omitempty` and an empty list is indistinguishable
  from an absent one on the wire.

## Spec conflicts settled in favour of the code

Recorded so the calls can be checked rather than rediscovered:

| Was written | What the code does |
|---|---|
| 004: strip is `Projects \| Sessions \| Tree`, with `Alt+S` | `Projects \| Tree`; only `Alt+P` and `Alt+T` exist |
| 004: "nothing separates one project from the next but the space" | a one-pixel rule does, as [042](042-sidebar-project-rows.md) says |
| 004: project page is a New session button over a Sessions archive tab | `Tasks \| Stats`, as [030](030-project-tasks.md) says |
| 006: a project's right strip is `Options \| Stats` | `Options \| Commits` |
| 001: the Codex adapter is a stub | it is fully implemented; only a live subscription turn is unrun |
| 002: seven tables | eleven, plus the columns added since |
| 005: three UI unit test files | four — `api.test.js` joined them |
| 015: five Settings sections | six; Server sits between Models and Shortcuts |
| 038: the file ended mid-sentence | rewritten whole |
