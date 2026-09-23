# Ignored files in the Tree

The Tree includes ignored files and draws them gray. Every directory puts its
folders first and its files after them; each group is sorted alphabetically by
name, regardless of ignore status. Sorting uses the browser's locale
comparison.

`GET /api/projects/:id/tree` returns `{files, ignored, dirs}`, plus
`truncated: true` when the folder holds more than one listing carries.
Git supplies tracked files through `git ls-files --cached`, untracked ones
through `git ls-files --others --exclude-standard`, and ignored files through
`git ls-files --others --ignored --exclude-standard`. Deleted tracked files
are removed. Empty directories are listed separately; see
[046](046-tree-file-actions.md).

Outside a repository, the filesystem walk includes dependency folders too.
Repository metadata directories named `.git` are skipped. This fallback
does not classify ignore rules, so its entries are not gray.

## The cap

One listing holds at most 200,000 entries (`maxTreeEntries`). Projects stay
inside it: the largest one measured holds 159,768 entries, nearly all of them
ignored dependencies. That is a 15 MB answer the pane builds in about 200 ms,
so real trees stay complete and a large dependency folder cannot hide ignored
files later in the list. The cap is for folders that are not projects. A
project on a home directory listed 9.6 million files, a 1 GB answer. It took
the server to 20 GB and the window to 17 GB, and the window never finished
reading it.

The cap is spent in order: tracked files, untracked files, ignored files,
then empty folders. Git prints untracked files before tracked ones when asked
for both, so they are separate calls, and a folder not yet in `.gitignore`
cannot push the project out. Each git listing is read line by line, and git is
stopped once it prints more than what is left, so millions of untracked or
ignored files cost no more than the cap. A listing that runs past git's time
limit is cut the same way. A cut listing names no empty folders, since it
cannot tell an empty folder from one whose files fell past the cut.

The walk outside a repository goes a level at a time. A folder too large to
list whole keeps its upper levels: a home directory shows its own folders
rather than the first files of `~/.cache`.

A truncated Tree says so above its rows, and Go to file, finding nothing, says
it searched only the listed part. It is not re-read on `files_changed` (see
[019](019-file-watching.md)); Tree actions and project switches still re-read
it.

The client combines the file lists. `buildTree` marks ignored files with
`ig`; a nonempty folder is gray when all its children are ignored.
All folders start collapsed. Expansion is saved per project across sessions;
clicking toggles either kind.

The fuzzy filter matches full paths, including ignored files, and preserves
alphabetical order and gray styling. A collapsed ignored folder shows its
match count during filtering. Filtering temporarily opens ordinary matching folders and preserves saved
expansion. Ignored folders open if saved as expanded or toggled in the filter. Clicking an ignored file opens it normally.

Regression tests cover listings beyond 20,000 entries, the order the cap is
spent in, the level-by-level walk, alphabetical order at multiple levels, and
ignored entries remaining visible and marked after filtering.
