# Settings sections and editable shortcuts

Enqueue is `Enter`; `Ctrl+Enter` sends. `Alt+Enter` no longer queues anything:
it was chosen because `Ctrl+Enter` looked taken, and it is not.

Every chord the app answers to now comes from one registry, `shortcuts.js`, and
the handlers ask it a question instead of spelling a key out —
`hit(e, "prompt.send")` rather than `e.key === "Enter"`. A chord is written the
way it reads, `Ctrl+Shift+T`, and compared on the physical `e.code`, so a
layout where Alt and a digit produce a different character still matches. The
modifiers are matched exactly, which is what lets `Ctrl+T` and `Ctrl+Shift+T`
be two bindings whose handlers can be tested in any order.

Most of them are editable. Click a chord in Settings and the next keystroke
becomes the binding; Escape cancels, and a per-row arrow restores that row's
default. Recording listens in the capture phase with propagation stopped, so
binding `Ctrl+P` records `Ctrl+P` instead of opening the launcher, and Escape
reaches the recorder rather than closing the dialog. Only what differs from a
default is written to `localStorage`, so a default that changes later still
reaches everyone who never touched it.

Recording replaces a whole binding. `New task` answers to both `Ctrl+N` and
`Ctrl+T` by default; rebinding it leaves one chord, and the row's reset brings
both back.

Four kinds of row are shown but not rebindable, marked `fixed` in the registry:
the two families (`Alt+1…9`, `Alt+A…H`), the divider arrows, and the editing
conventions the textareas answer to themselves (Tab, undo/redo, Shift+Enter).
A family is one binding standing for nine actions, and an editor that does not
undo with `Ctrl+Z` is a worse editor.

A chord bound to two actions is not refused — it is labelled, on both rows,
with the other action's name. Which one runs is whichever handler asks first,
and that is not something to find out by accident.

The dialog grew a rail to hold all this: **General**, **Projects**,
**Appearance**, **Models**, **Server**, **Shortcuts** — General is
[017](017-general-settings.md), Projects lists what has been archived
([041](041-project-archiving.md)), and Server is
[043](043-exposed-server.md) — with a filter above them. The filter fuzzy-matches every row in every section — a palette name, a
model, a shortcut and its keys — and each section reports how many it kept, so
the count beside a section says where the answer is before you click.
Filtering into a section that has no matches moves to one that does. Every
pane stays mounted, which is what keeps those counts live; they are small
enough that this costs nothing.

Matching is per field rather than over one joined string: a subsequence match
against a label and its keys run together is loose enough to hit almost
anything, and `fuzzyAny` scores the best field instead.

The standalone shortcut dialog is gone. The keyboard button under the sidebar
and the launcher's new *Keyboard shortcuts* entry both open Settings on the
Shortcuts section, so there is one list rather than two that can disagree —
and it is now generated from the registry the handlers read, which is why it
cannot disagree with them either.
