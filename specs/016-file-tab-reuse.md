# One file tab a click reuses

## Outcome

Reading through files no longer fills the strip. A click on a file — in the
Tree, in Changed, or on a commit's file list in Logs — opens it in the
project's *temporary* tab, and the next file clicked takes that tab over: same
place in the strip, same `Alt` number, one tab. A double click opens the file
to keep, and so does typing in it. Kept tabs are only closed by hand.

`temp` on the file tab is the whole state. `tempFileIn` finds the one a click
reuses, and `swapTab` puts the new tab at the old one's index rather than
closing and appending, so the strip's length and order hold still under the
pointer. A temporary tab is drawn in italic and says so in its tooltip.

There is at most one per project, because the strip is per project: the tab a
click takes over is one the user can see.

`editFile` clears `temp`, so a replacement can never eat edits — the file
being edited stopped being the reusable one at the first keystroke. Taking the
edit back does not hand the tab over again.

`temp` is saved with the open tabs, so a reload finds temporary and kept tabs
as they were left. Tabs saved before the flag existed restore as kept.

## Choices

**Promote on edit rather than refuse to replace.** "Reuse unless it has
edits" would leave a tab that is both the reuse slot and not reusable, and the
close-with-unsaved-edits dialog would need a second caller. Clearing the flag
on the first keystroke keeps one invariant: a temporary tab is never dirty.

**A replaced tab is not remembered as closed.** `Ctrl+Shift+T` walks tabs the
user chose to close. Pushing every file glanced at onto that stack would bury
the one close worth undoing.

**Pinning names the tab by id, not by object.** A double click is two clicks
and so two `openFileIn` calls, the second arriving while the first is still
fetching. Both resolve the open tab by id — `selectOpenFile`, checked before
the fetch and again after it — so whichever call inserted the tab, the double
click still pins it. In practice the first click opens the temporary tab and
the double click keeps that same tab, which is why a double click does not
leave the file previewed alongside.

**`temp`, not `preview`.** A file tab already has a Preview view for markdown
and HTML in `mode`. Two meanings of the word in one object read as a bug.

## Validation

Browser-checked on a fresh database against this repo, with no console or page
errors. Clicking `go.mod`, then `go.sum`, then `Makefile` in the Tree left one
italic tab that changed its name each time. A double click on `LICENSE` took
the italic off; the next click opened a second, italic tab beside it. Typing
one character into that tab took its italic off too, and the next click opened
a third. Clicking a kept file's row selected its tab and left it kept.

Clicking a file in Changed, then one in the Tree, then one under a commit in
Logs reused the same single tab across all three, and a double click on the
commit's file kept it. After a reload the kept tab came back kept and the
italic one came back italic.
