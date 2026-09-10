# Ignored files in the Tree

The Tree lists every file in the project, not only the ones git would track.
What `.gitignore` covers is drawn grey, sorted below what it does not, and its
folders start shut. Everything else about the pane is unchanged: the filter is
the same fuzzy match over the whole path, the highlighting is the same, a click
opens the same temporary tab, and the count beside the filter counts every
match.

`GET /api/projects/:id/tree` answers `{files, ignored}` instead of one array.
`files` is `git ls-files --cached --others --exclude-standard` as before;
`ignored` is `git ls-files --others --ignored --exclude-standard`. They are kept
apart for two reasons. The pane needs to know which is which, and the cap has
to be spent in the right order: in agenttik's own repository that is 140 files
against **13,789** ignored ones, so a single capped list would be nothing but
`node_modules` and every real file would fall off the end. `files` takes the cap
first and `ignored` gets what is left of the 20,000.

`buildTree(paths, ignored)` marks a node `ig`. A file is ignored when it is in
the set; a folder is ignored when everything inside it is, which is the only
definition available — git ignores files, not folders — and reads correctly:
`web/dist` is *not* grey here, because `.gitignore` keeps `web/dist/.gitkeep`
tracked, and the folder really does hold a tracked file. Sorting gained one key
in front of the existing two: `ig`, then folders before files, then name. A
folder therefore reads as its own contents first and what was generated into it
after.

In the pane, the remembered set of hand-toggled folders now means "closed" for
a project folder and "open" for an ignored one — `toggled.has(path) === !!n.ig`
— which is the whole of the difference between them: one is occasionally
closed, the other occasionally opened. Clearing the set on a filter change
still puts each kind back the way it starts. A shut ignored folder is the one
place a filter's hits are out of sight, so while a filter is typed it carries
the number of matches inside it.

## Choices

**Ignored folders start shut, and stay shut through a filter.** This is the
whole design, and without it the feature is unusable rather than merely slow.
Measured on this repository: building the tree from 13,929 paths takes 48ms and
filtering it 3–7ms, so the JavaScript was never the problem. The DOM and the
eye are. Folders start *expanded* in this pane, so listing ignored files
plainly would draw 14,000 rows on open; and a filter clears the collapsed set,
so typing `wesst` — 30 matches today — would draw **12,100**, of which 30 are
the answer. Shut, the same filter draws 48 rows: the real hits, plus one grey
row per ignored folder carrying its count. Expanding one is then the user's own
choice, and it opens one level at a time, each ignored child shut with a count
of its own.

**Grey rather than a toggle.** A "show ignored files" switch would have been
less code and worse: the reason to want these files is that an agent just wrote
one, which is exactly when you do not know to go and turn the switch on.

**The cap is split, not shared.** `listIgnored` takes `maxTreeEntries - len(files)`.
The alternative — one list, one cap — silently loses real files behind a
dependency folder, and the loss is invisible because a truncated tree looks
exactly like a small project.

**Nothing is grey outside a repository.** The hand-walked fallback has no
patterns to test against, so it skips the dependency folders it always skipped
and returns an empty `ignored`.
