# Project logs

The right-hand panel carries a **Commits** pane beside Changed for a task and
beside Project Options and Changed for a project — the history of the branch the project is on.
A branch autocomplete sits above a commit filter box. A reserved message area
below the branch holds operation results and errors; it does not show the head hash.
The autocomplete lists local branches, ranked by the same fuzzy subsequence
search as the commit filter. Selecting a branch runs Git switch; checkout
conflicts are reported in the pane.

To its right, buttons with tooltips pull (down arrow), push (up arrow), and
clean merged branches (brush). Pull is fast-forward only. Push sends
only HEAD to the current branch's configured upstream; a missing upstream is
reported as an error. Controls are disabled while an action runs and remote
actions have a two-minute timeout. The log, changes, and tree refresh afterward.

## Merge and rebase

Below the current branch selector, a **Merge / rebase** disclosure starts collapsed.
Expanding it shows the operation and destination selectors, a right-aligned action
button, and the help text. The operation selector reads **merge into** or
**rebase into**, followed by a destination autocomplete. Every other local branch
is selectable; neither side is restricted to main/master. The four-point sparkle
button reads **Merge & solve conflicts** or **Rebase & solve conflicts**. A tooltip on
the button explains temporary stashing and AI conflict resolution using the model
in Settings → Models. Help text below identifies which branch changes.

Merge checks out the destination and merges the selected source into it. Rebase
keeps the source checked out and replays its commits onto the destination,
rewriting the source only. Both automatically stash changed and untracked
files, then restore them in the resulting checkout without confirmation. Staged
and unstaged changes retain their original split, including partially staged
files; untracked files remain untracked.
The button tooltip includes “Changed files will be stashed temporarily.”
Paused operations retain their stash across retries and app restarts; completion,
abort, and failures before an operation starts restore it automatically. If
restoration conflicts, the error identifies the retained stash for manual recovery.
Existing user stashes are preserved. Git refuses destinations checked out in another worktree.
Clean operations run without a model. Nothing pushes.

Conflicts use the saved automatic-action model (see [069](069-automatic-action-models.md)).
The model gets an isolated workspace request to inspect and edit conflict paths.
The server stages those paths and continues Git itself. It verifies unchanged HEAD
and operation metadata, no unrelated edits, no remaining conflict markers, and no
unstaged edits before continuing. Rebases repeat this for each conflicted commit.
Each request allows ten minutes of resolution and at most 100 conflict rounds;
individual Git commands are bounded too. Model errors and timeouts leave Git's
operation available for recovery instead of reporting success.

An in-progress merge/rebase replaces the operation controls with **Retry solving
conflicts** and **Abort**. These also appear after reopening the pane. Retry can
continue a manually staged resolution; Abort uses Git's normal abort behavior.
Controls are disabled while a request runs. Branch, staging and commit requests
share a checkout lock across windows; this does not lock out external Git tools.

Cleanup immediately deletes local branches whose tips are ancestors of either
local main or local master. It preserves main, master, the current branch,
and every branch checked out in another worktree. If neither local base exists,
no local branches are deleted. The pane reports deleted names or that no local
branches qualified. Squash merges are not ancestry merges.

After local cleanup, Git fetches and prunes heads from each configured remote.
Branches merged into that same remote's main or master appear in a confirmation
dialog. Remote main, master, and HEAD are excluded. Cancel or dismiss keeps all
remote branches; local cleanup has already completed. Confirm deletes all listed
branches from their remotes. The server fetches and validates the entire selection
again before deletion, and explicit tip leases reject concurrent branch updates.
Newly eligible branches require another confirmation. Deletion errors report any
branches already removed. Fetch failures leave local cleanup intact and show an
error. Remotes with different fetch and push destinations are rejected because
the fetched history cannot safely establish eligibility at the push destination.

A row is the commit subject with a small `[N]` chip for affected files, and
under it, in grey, what identifies it:

```
Read the subscription allowance each CLI reports [7]
1323b7ae · Pau Sanchez · 2026-09-09 16:46
```

Hovering the commit subject shows the full commit message, including its body
and paragraph breaks, in a native tooltip in both simple and graph views.

The hash is abbreviated to eight characters. The date is `YYYY-MM-DD HH:MM`,
formatted by git in the zone the commit was made in — git knows that offset
and the browser does not. That line wraps rather than truncating in a narrow
panel; hiding half of it is worse than using two lines.

The current checkout's commit has a primary background and left border in both
views. Branch names appear as primary chips on a separate, wrapping line below
the metadata in both views; no branch name appears beside the date.

The **Show commit graph** toggle beside the filter defaults off and stays selected
while navigating within the app. Simple mode shows the current branch history.
Graph mode includes local branches, remote branches and tags, ordered with children
before parents. Colored lines and square nodes show ancestry, splits and merges;
merge nodes are hollow. Lines continue beside expanded files. Branch heads and tags
appear on the chip line: branches use primary, tags secondary. Tags are shown
only in graph mode.
Secondary uses violet, or pink when the primary accent is violet.
Annotated and lightweight tags are supported; symbolic remote aliases are omitted.

Counts include additions, modifications, deletions, renames and binary files.
Merge counts, expanded files and diffs compare against the first parent. Empty
commits show `[0]`. Counts come from a single batched log, without requests per row.

Typing filters what has already been fetched, with the same subsequence match
the Tree pane uses, against subject, author and hash as one string — so a hash
prefix, a name, or the words of a message all reach the same commit. Only the
subject is highlighted, so letters that matched the author do not light up
arbitrary characters in the title. Simple search ranks by match score; graph
search preserves topology and connects visible ancestors through hidden commits.

Clicking a row expands the files that commit touched, each with its status
letter and its own `+`/`−` counts. Clicking a file opens it as a tab, showing
that commit's diff.

Commits with more than 1,000 files initially render only the first 250 files.
The **See 250 more** button adds 250, then offers 500 more, then 1,000 more
per click until all files are visible. The last button shows the remaining
count and disappears after those files are shown. File order is preserved.
Reopening a commit starts from the initial batch; commits with at most 1,000
files show the full list immediately. The full file response is still fetched
and cached once; only the visible prefix is rendered to keep opening large
commits responsive.

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
| GET | `/api/projects/:id/log?limit=&graph=` | `{branch, branches[], head, operation, commits[]}` — up to 500 commits in topological order; graph includes all branch/tag histories |
| POST | `/api/projects/:id/branches/:action?repo=` | `{branch, target?}`; switch/pull/push return 204, clean returns `{deleted[]}`; check-remote returns `{remotes[{remote,branch,tip}]}`; clean-remote accepts `{branch,remotes[]}` and returns `{deleted[],error?}`; merge/rebase/continue/abort return `{message}` |
| GET | `/api/projects/:id/commit?hash=` | `{hash, files[{path,status,additions,deletions,binary}]}` |
| GET | `/api/projects/:id/commit/diff?hash=&path=` | the same `{path, diff, partial}` a working-tree diff answers |

Each commit includes `hash`, `subject`, `message` (full subject and body), `author`, `date`, `parents[]`,
`branches[]`, `tags[]` and `fileCount`. The log is one `git log --shortstat` with
a record-separated pretty format and first-parent merge diffs. One
`for-each-ref` call attaches branch and tag names. The file
list is `git show --numstat` plus `--name-status`, which override each
other and so cannot be one call. A commit's files are fetched once per commit
— history does not change underneath — and the whole log is re-read on watched
filesystem changes and when a turn ends. Branch switches during a task or in a terminal update the current
branch, available branches and history without reopening the pane.

`hash` is matched against `^[0-9a-fA-F]{4,40}$` rather than merely escaped: anything else could
be read as a flag. Switch accepts only validated, existing local branch names.
Other branch actions reject requests if the current branch no longer matches.
Paths go through the same `resolveInRoot` check as every other file route. A folder that is not a repository, and one with no commits
yet, both answer with an empty log rather than an error.

A rename's destination is the file that exists, and git writes the pair either
whole (`old => new`) or factored (`dir/{a => b}/file`) with either side
possibly empty — `web/{ui => }/index.html` is a move up one level. The
destination is cleaned rather than concatenated; without that it reads
`web//index.html`, matching neither the file on disk nor the path
`--name-status` reports its status letter under.
