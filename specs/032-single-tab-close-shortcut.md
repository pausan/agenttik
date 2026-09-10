# Single-tab Ctrl+W

## Outcome

`Ctrl+W` closes exactly the active tab. It closes a file tab without closing
its session, and closes a session without closing its file tabs. Project and
schedule tabs follow the same rule. Holding the chord does not close another
tab, and the browser never receives the chord.

The tab strip's `×` keeps its existing cascade behavior: closing a session
there also closes files opened from it.

## Implementation

The global key handler passes `S.tab.id` to `closeTabOnly` and ignores repeated
`keydown` events. `closeTab` accepts a cascade flag so the unsaved-file check,
closed-tab history, and tab removal all use the same scope. A keyboard close
remembers only the tab it removed; the `×` still remembers its dependents for
reopening.

## Validation

The change was reviewed with static source checks and `git diff --check`.
Tests were not run, per the prototype repository instructions.
