# Settings sections and editable shortcuts

Enqueue is `Enter`; `Ctrl+Enter` sends. On macOS the primary modifier is `Cmd`,
so the same binding is `Cmd+Enter`. `Alt+Enter` no longer queues anything: it
was chosen because the modified Enter chord looked taken, and it is not.

Browser shortcuts come from one registry, `shortcuts.js`, and
the handlers ask it a question instead of spelling a key out —
`hit(e, "prompt.send")` rather than `e.key === "Enter"`. A chord is written the
way it reads, `Ctrl+Shift+T` on Windows and Linux or `Cmd+Shift+T` on macOS,
and compared on the physical `e.code`, so a
layout where Alt and a digit produce a different character still matches. The
modifiers are matched exactly, which is what lets the primary-modifier `T` and
primary-modifier `Shift+T`
be two bindings whose handlers can be tested in any order.

The desktop global show/hide shortcut lives in General → Desktop tray,
because the native shell registers it before the UI loads; see
[060](060-desktop-tray.md).

`Ctrl+Alt+P` moves to the next local profile; `Ctrl+Alt+Shift+P` moves to the
previous one. Both wrap in the saved profile order, ignore held-key repeats,
and use Ctrl on every platform. Switching uses the normal profile reload and
unsaved-file warning.

Most browser shortcuts are editable. Click a chord in Settings and the next keystroke
becomes the binding; Escape cancels, and a per-row arrow restores that row's
default. Recording listens in the capture phase with propagation stopped, so
binding `Ctrl+P` (or `Cmd+P` on macOS) records that chord instead of opening
the launcher, and Escape
reaches the recorder rather than closing the dialog. Only what differs from a
default is written to `localStorage`, so a default that changes later still
reaches everyone who never touched it.

Recording replaces a whole binding. `New task` answers to both `Ctrl+N` and
`Ctrl+T` by default, using `Cmd` in place of `Ctrl` on macOS; rebinding it
leaves one chord, and the row's reset brings both back. Saved chords recorded
as `Meta` by earlier versions are shown and matched as `Cmd` on macOS.

Four kinds of row are shown but not rebindable, marked `fixed` in the registry:
the two families (`Alt+1…9`, `Alt+A…H`), the divider arrows, and the editing
conventions the textareas answer to themselves (Tab, undo/redo, Shift+Enter).
A family is one binding standing for nine actions, and an editor that does not
undo with the platform's usual primary modifier is a worse editor.

A chord bound to two actions is not refused — it is labelled, on both rows,
with the other action's name. Which one runs is whichever handler asks first,
and that is not something to find out by accident.

Settings has a collapsible navigation tree, in this order:

- General: its own pane, with Appearance, Profiles, Projects and Shortcuts below it.
- Orchestrator.
- Providers: Subscriptions and API Providers.
- Models, Server, Help and About.

General opens by default. Parent chevrons collapse their children; selecting a
child through an external shortcut opens its parent. Providers selects
Subscriptions. On phones the groups form a bounded, horizontally scrolling rail.
The icons are bundled from both Vue and JavaScript sources for offline use.

The filter fuzzy-matches rows in every pane. Matching children keep their parent
visible and expanded, parent counts include matching descendants, and a selected
pane with no matches moves to the first matching pane. Panes mount on first visit and stay mounted until Settings closes, preserving
unsaved fields. Entering a filter mounts all panes to populate search counts. API setup and connected-model filtering are described in
[078](078-api-providers.md).

Matching is per field rather than over one joined string: a subsequence match
against a label and its keys run together is loose enough to hit almost
anything, and `fuzzyAny` scores the best field instead.

The standalone shortcut dialog is gone. The keyboard button under the sidebar
and the launcher's new *Keyboard shortcuts* entry both open Settings on the
Shortcuts section, so there is one list rather than two that can disagree —
and it is now generated from the registry the handlers read, which is why it
cannot disagree with them either.

`Ctrl+Shift+P` opens **Command Palette**, the fuzzy launcher for navigation
and actions. It includes `Switch to profile: <name>` for each other local profile,
refreshing the profile list on open. Selecting one reloads the window in that
profile; see [067](067-local-profiles.md). `Ctrl+P` opens **Go to file**, searching full relative paths in
only the current project's live file listing, including ignored files.
Matching is case-insensitive and accepts letters in order; the best 100
matches are shown with matching letters highlighted. Arrow keys select a
result, Enter or a click opens it, and Escape dismisses the dialog.
Text opens in Edit; images and fonts open in Preview. This also applies to
existing tabs and uses the same editor-first default as Tree selections.
File search uses the usual temporary file tab and preserves unsaved edits.
