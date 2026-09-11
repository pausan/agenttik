# Making, renaming and deleting files from the Tree

The Tree pane is a file manager as well as a listing. A right click anywhere in
it opens one menu — **New file**, **New folder**, then **Rename** and
**Delete** — and **F2** renames the row the keyboard is on without going
through the menu at all.

Each of the four asks first. Three ask for a name, the fourth asks for a yes;
all four are `TreeActionModal`, because three of them differ only in their
wording and a component each would have been four copies of the same dialog.

## What the click was aimed at

One menu serves the whole pane, aimed by the `contextmenu` event on its way to
the trigger — the same single-listener shape `LogsPane` uses for its commits. A
menu component per row would be one per file in the project, and only the row
under the pointer has anything to offer.

The row aimed at decides where a new entry lands: **inside** the folder clicked,
**beside** the file clicked, and at the top of the project when the click missed
every row. The dialog says which, under the name box. A click that missed also
has nothing to rename or delete, so those two grey out.

A name may be a path — `sub/thing.go` — which makes the file in a folder that
need not exist yet, and on a rename moves it. Folders above what was just made
are opened, because a new file inside a collapsed folder is otherwise created
into thin air.

## What happens to the tabs

Open tabs are kept in step by hand, because a file tab's id carries its path:

- A **renamed** file's tab follows it. The id, the path and the label are
  rewritten, and so is whatever pointed at the old id — the active tab, and the
  project's last-tab memory. Unsaved edits survive the move. The text is the
  same bytes it was, but a diff is against a path — the move is part of the new
  one — so a tab left on Diff re-reads it.
- A **deleted** file's tab closes: forced, because there is nothing left to
  save, and unremembered, because Ctrl+Shift+T could only fail on a file that is
  gone.
- A folder takes everything under it, so both walk the subtree rather than the
  one path.
- A tab showing a file **at a commit** is left alone. History did not move.

A new file opens as it is made, pinned — it was made to be typed into, and a
temporary tab would be taken over by the next file clicked.

## Folders that hold nothing

The tree is built from file paths, so a folder with nothing in it cannot be
inferred from the listing: New folder would have appeared to do nothing at all.
`GET /api/projects/:id/tree` therefore answers a third list, `dirs`, and
`buildTree(files, ignored, dirs)` adds those as nodes with no children.

`emptyDirs` finds them with one read per folder the project actually has. At
each folder it asks the listing two questions, both binary searches over the
sorted lists rather than a set of every ancestor of every path:

| Is a listed file under it? | Is an ignored file under it? | What happens |
|---|---|---|
| yes | — | walked, as a folder of the project |
| no | yes | skipped, and not named: `node_modules` is already in the tree through its own files |
| no | no | named as empty, and walked — it holds nothing, so that is cheap |

So the walk never enters a dependency folder, and a folder made two deep is
still found. `dirs` takes what is left of the 20,000-entry cap after `files` and `ignored`,
following the same rule as those two —
see [027](027-ignored-files-in-the-tree.md).

## The endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/projects/:id/entry` | `{path, dir}` — an empty file, or a folder |
| POST | `/api/projects/:id/entry/rename` | `{path, to}` |
| DELETE | `/api/projects/:id/entry?path=` | a file, or a folder with everything in it |

All three answer `{path, dir}`: what now exists, and whether it is a folder.
Missing parents are created on the way, on both the create and the rename.

They resolve their path through `resolveEntry`, which is deliberately **not**
the `resolveInRoot` the reading endpoints use. That one follows the path's own
symlink, which is right for reading a file through a link and wrong here: the
row in the tree stands for the link, so deleting it must unlink it rather than
destroy what it points at. `resolveEntry` resolves the nearest existing
**parent** instead — which is also all an entry that does not exist yet leaves
to check — and refuses the path if that parent lands outside the project. On top
of the traversal and absolute-path rules it shares with `resolveInRoot`, it
refuses the project folder itself and anything under `.git`.

## Choices

**A modal, not an inline editor.** Renaming in place reads better and costs a
phantom row in a list that is rebuilt from the server's listing on every
keystroke of an agent's writing. The dialog is four lines of state and cannot
be knocked out from under the user by a refresh.

**Delete is permanent, and says so.** There is no trash here and no undo; the
dialog names the path, says a folder goes with everything in it, and puts the
word Delete on a red button. In a repository the file is still in git, which is
the only safety net that was going to exist.

**F2 is not in the shortcut list.** It is already bound there to renaming the
current *task* or scheduled job. The tree's row handles the key itself and stops it before the
window listener sees it, so the same key renames whatever the keyboard is on. A
second F2 entry in Settings would be reported as a conflict by
`keyConflicts()` — which is exactly what it is not.

**The empty folder is the server's problem, not the pane's.** The pane could
have remembered folders it had just made and drawn them until the listing
caught up. That folder would then vanish on a reload, and be invisible to a
second window on the same project. A listing that answers what is on disk is
answerable by anything that asks it.
