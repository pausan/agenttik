# Project logs

The right-hand panel carries a **Commits** pane beside Changed for a task and
beside Options for a project — the history of the branch the project is on.
The branch name and the short head hash sit above a filter box; under it, one
row per commit.

A row is the commit subject, and under it, in grey, what identifies it:

```
Read the subscription allowance each CLI reports
1323b7ae · Pau Sanchez · 2026-09-09 16:46
```

The hash is abbreviated to eight characters. The date is `YYYY-MM-DD HH:MM`,
formatted by git in the zone the commit was made in — git knows that offset
and the browser does not. That line wraps rather than truncating in a narrow
panel; hiding half of it is worse than using two lines.

How much a commit moved is not shown. It was, as `13 files · 408 changes`,
but a size next to every subject reads as a ranking the pane does not mean,
and it cost a `--shortstat` — git diffing all 500 commits — on a log that is
re-read after every turn.

Typing filters what has already been fetched, with the same subsequence match
the Tree pane uses, against subject, author and hash as one string — so a hash
prefix, a name, or the words of a message all reach the same commit. Only the
subject is highlighted, so letters that matched the author do not light up
arbitrary characters in the title.

Clicking a row expands the files that commit touched, each with its status
letter and its own `+`/`−` counts. Clicking a file opens it as a tab, showing
that commit's diff.

Right-clicking a file in Changed or in an expanded commit offers **Show in
tree**. That switches the left sidebar to Tree, clears any Tree filter, opens
the folders above the file, selects and scrolls to its row, and opens the
working-tree file in the editor.

Right-clicking a commit row offers **Copy hash**. Right-clicking one of its
expanded files offers both **Copy hash** and **Show in tree** — that file
belongs to the commit. One menu serves the whole list rather than one per row,
which would be 500 of them for a full log: the row under the pointer is read
off the event on its way to the trigger, and a click that reaches no row leaves
the items greyed rather than copying or showing whatever was aimed at last.

## Commit file tabs

A commit's file reuses the ordinary file tab, with the revision in its id, so
the same path can be open once for a commit and once for the working tree. It
is fixed to `Diff`: there is nothing to edit in a revision already made, and
its text is not what is on disk, so offering Edit or Preview would only show
the wrong thing. The header carries `@ <hash>` and the status bar says
`committed` instead of `read only`. The tab is saved and restored like any
other, revision included.

## HTTP

| Method | Path | Answers |
|---|---|---|
| GET | `/api/projects/:id/log?limit=` | `{branch, head, commits[]}` — up to 500 commits, newest first |
| GET | `/api/projects/:id/commit?hash=` | `{hash, files[{path,status,additions,deletions,binary}]}` |
| GET | `/api/projects/:id/commit/diff?hash=&path=` | the same `{path, diff, partial}` a working-tree diff answers |

The log is one `git log` with a record-separated pretty format; the file
list is `git show --numstat` plus `--name-status`, which override each
other and so cannot be one call. A commit's files are fetched once per commit
— history does not change underneath — and the whole log is re-read when a
turn ends, because a turn that commits has changed history as well as the tree.

`hash` is the only client value handed to git as a revision, so it is matched
against `^[0-9a-fA-F]{4,40}$` rather than merely escaped: anything else could
be read as a flag. Paths go through the same `resolveInRoot` check as every
other file route. A folder that is not a repository, and one with no commits
yet, both answer with an empty log rather than an error.

A rename's destination is the file that exists, and git writes the pair either
whole (`old => new`) or factored (`dir/{a => b}/file`) with either side
possibly empty — `web/{ui => }/index.html` is a move up one level. The
destination is cleaned rather than concatenated; without that it reads
`web//index.html`, matching neither the file on disk nor the path
`--name-status` reports its status letter under.
