# Two ways to start a project

Add project opens on a segmented **Folder | Multiple Git Repos** switch. Both end in
the same thing — one project on one folder — and differ only in whether that
folder is filled in on the way.

**Folder** is the original: pick a folder that already exists, optionally name
the project, Add.

**Multiple Git Repos** picks a *base folder* and clones one or more repositories into
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

Each repository has its own input row. Typing in the final row immediately
creates an empty next row, even while validation is pending or fails. Pasting
whitespace-separated URLs creates one row per URL and a trailing empty input.
A non-empty row checks its remote after a short pause by asking git for its
refs, not history. A spinner, tick, or cross appears at the input's right edge.
Checks for edited or deleted rows cannot replace the current result. Empty
rows are ignored. Each row has a delete button beside its input; long URLs
and error messages stay inside the dialog, and long lists scroll.

GitHub and GitLab web links become their SSH remotes; ordinary HTTPS, git, and SSH remotes remain accepted. HTTPS clone URLs ending in `.git` stay HTTPS. Absolute paths to existing local directories are accepted too; Git verifies they are repositories. Relative paths and files are rejected. Add project stays disabled while a check is running or any non-empty row is invalid. It starts sequential clones for the checked rows and reports progress. A failed clone becomes a broken row and prevents adding the project until it is fixed or deleted. The dialog can be minimized during this work; its compact progress button restores it without interrupting the queue.

At least one validated repository is required. Empty inputs do not count, so
the mode cannot accidentally add a base folder with no repositories.

The destination is git's own rule — the last path segment without a trailing
`.git` — under the base folder. A name already taken is refused before git
runs, which is also what keeps a second clone of the same URL from landing
twice.

## Choices

**No new schema, and no stored mode.** A project is still one row with one
path. Multi-repo is a way of *filling* a folder, not a kind of project, so
the Tree and the agent's working directory use that base folder. The clones
are simply what the folder contains. A project made this way is indistinguishable
from one made by cloning the repositories by hand and adding the folder.

**Git panes use the selected repository.** The Workspace header on the right
selects among repositories inside the current project when there are multiple;
a single repository is used automatically. Changed and Commits show that
repository's status and history. See [053](053-workspace-repositories.md).
The Tree still lists the whole project, using its non-repository fallback for
a base folder. That fallback skips dependency folders and Git internals, but
does not classify ignored files. The agent runs from the base folder and can
run Git in whichever checkout it is working in.

**The clone URL is an allowlist, not an escape.** git's transports include
`ext::`, which runs a shell command the URL chooses. agenttik has no login and
its server can be put on the network ([043](043-exposed-server.md)), so
`remoteURL` matches only `https://`, `http://`, `ssh://`, `git://` and git's
scp-like `user@host:path`, with an optional `user@` or `token@` before the
host. The host must start with a word character for the same reason one step
down: a host beginning with `-` is an option to whatever `ssh` git calls. The
URL also reaches `git clone` after a `--`, so it cannot be read as a flag.
Absolute local paths bypass the URL pattern only after a directory check; they
also reach Git after `--`.

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
