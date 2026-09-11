# Two ways to start a project

Add project opens on a segmented **Folder | Git repos** switch. Both end in
the same thing — one project on one folder — and differ only in whether that
folder is filled in on the way.

**Folder** is the original: pick a folder that already exists, optionally name
the project, Add.

**Git repos** picks a *base folder* and clones one or more repositories into
it. The project points at the base folder, so every checkout under it is one
working directory the agent sees at once: it can read across the repositories
and change more than one in a turn.

Either way the picker now has a **New folder** button, so a project can start
somewhere that did not exist a moment ago without leaving for a shell.

## New folder

The folder-plus button sits at the right of the picker's breadcrumb bar, apart
from the crumbs so it stays put while they scroll. It opens a naming row at the
top of the listing; Enter creates the folder inside the one on screen and walks
into it, exactly as clicking an existing folder would, which leaves it selected.
Escape or clicking away closes the row.

A name that cannot be used — one already taken, a read-only parent — is
reported under the listing in the picker's own error line and the row stays
open on the name to change. `POST /api/fs/dir` takes `{path, name}` and makes
one folder: `name` is a single segment, so `.`, `..` and anything containing
`/` or `\` are refused rather than quietly landing somewhere else.

Because this is `FolderPicker`, the Options pane's repoint button
([040](040-editable-project-path.md)) gets the button too.

## Cloning

Each repository is cloned as it is named, not when the project is added. A
clone takes as long as it takes, and its failures — a typo, a private
repository, no network — are worth seeing one at a time while the dialog is
still open and the base folder can still be changed.

Enter in the Repository field, or the Clone button, appends a row and starts
`POST /api/fs/clone` with `{path, url}`. The row shows the URL and a spinner
while it runs, then the folder git made and a tick, or a cross and git's own
last line. Clones run alongside each other, so a large repository never holds
up the next one; **Add** is disabled while any is still going. A failed row
carries an `×` that drops it from the list — a successful one does not, since
the clone is on disk inside the folder the project is about to point at, and
forgetting it here would only make the list lie. A successful clone re-lists
the folder browser above, so the new checkout appears there as the folder it
now is.

Add refuses in this mode until at least one clone has succeeded: with none,
the mode is Folder with extra steps, so it says so rather than silently
behaving like it.

The destination is git's own rule — the last path segment without a trailing
`.git` — under the base folder. A name already taken is refused before git
runs, which is also what keeps a second clone of the same URL from landing
twice.

## Choices

**No new schema, and no stored mode.** A project is still one row with one
path. Multi-repo is a way of *filling* a folder, not a kind of project, so
nothing downstream — the Tree, the Changed pane, `git` in the Commits pane,
the working directory a turn runs in — has to learn about it. The clones are
simply what the folder contains. A project made this way is indistinguishable
from one made by cloning the repositories by hand and adding the folder,
which is the point.

**The git-backed panes read the project root, which a base folder is not.**
`isGitRepo` looks for `.git` at the root, and a folder holding checkouts has
none, so Commits and Changed both answer empty rather than erroring, and the
Tree falls back to the hand-walk `listFiles` already keeps for a project that
is not a repository. Two consequences worth knowing before choosing this mode:
there is no one history or one working-tree status to show — each checkout has
its own, and neither pane aggregates — and the hand-walk has no notion of
`.gitignore`, so build outputs are listed and nothing comes back grey. Only
`skipDirs` — `.git`, `node_modules`, `.venv`, `vendor`, `__pycache__` — is
skipped, and the listing stops at 20,000 entries. None of this is new; it is
what every non-repository project has always done. This mode just makes it the
normal case rather than the odd one, and the agent itself is unaffected: it
runs `git` in whichever checkout it is working in.

**The clone URL is an allowlist, not an escape.** git's transports include
`ext::`, which runs a shell command the URL chooses. agenttik has no login and
its server can be put on the network ([043](043-exposed-server.md)), so
`remoteURL` matches only `https://`, `http://`, `ssh://`, `git://` and git's
scp-like `user@host:path`, with an optional `user@` or `token@` before the
host. The host must start with a word character for the same reason one step
down: a host beginning with `-` is an option to whatever `ssh` git calls. The
URL also reaches `git clone` after a `--`, so it cannot be read as a flag.

**A clone never waits on a terminal this process does not have.**
`GIT_TERMINAL_PROMPT=0` with empty `GIT_ASKPASS` and `SSH_ASKPASS` turns a
credential prompt into an immediate failure, so a private repository says
"could not read Username" in its row instead of hanging until the 10-minute
timeout. That timeout is far longer than the 5 seconds the read-only git calls
get: this one talks to a remote over a repository that may be large.

**A failed clone leaves nothing behind.** git killed partway leaves a partial
folder, and the name has to be free for the retry, so the destination is
removed on failure. Nothing of the user's is at risk: the same path was
checked to be absent before git ran.

**The dialog reports inline, like the folder field already did.** Every
failure here — a rejected URL, a refused folder name, a clone that did not
work, Add on a folder another project holds — lands next to the thing that has
to change rather than in a toast behind the dialog, which is the rule
[040](040-editable-project-path.md) set for this dialog.
