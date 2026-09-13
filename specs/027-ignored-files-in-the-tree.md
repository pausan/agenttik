# Ignored files in the Tree

The Tree includes ignored files and draws them gray. Every directory puts its
folders first and its files after them; each group is sorted alphabetically by
name, regardless of ignore status. Sorting uses the browser's locale
comparison.

`GET /api/projects/:id/tree` returns `{files, ignored, dirs}`.
Git supplies tracked and untracked files through
`git ls-files --cached --others --exclude-standard`, and ignored files through
`git ls-files --others --ignored --exclude-standard`. Deleted tracked files
are removed. Neither listing has an entry cap, so a large dependency folder
cannot hide ignored files later in the list. Empty directories are listed
separately; see [046](046-tree-file-actions.md).

Outside a repository, the filesystem walk includes dependency folders too.
Repository metadata directories named `.git` are skipped. This fallback
does not classify ignore rules, so its entries are not gray.

The client combines the file lists. `buildTree` marks ignored files with
`ig`; a nonempty folder is gray when all its children are ignored.
All folders start collapsed. Expansion is saved per project across sessions;
clicking toggles either kind.

The fuzzy filter matches full paths, including ignored files, and preserves
alphabetical order and gray styling. A collapsed ignored folder shows its
match count during filtering. Filtering temporarily opens ordinary matching folders and preserves saved
expansion. Ignored folders open if saved as expanded or toggled in the filter. Clicking an ignored file opens it normally.

Regression tests cover listings beyond 20,000 entries, alphabetical order at
multiple levels, and ignored entries remaining visible and marked after filtering.
