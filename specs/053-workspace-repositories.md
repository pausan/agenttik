# Workspace repositories

The right inspector is titled **Workspace**. The left sidebar contains
Projects and Tree, with project rows as its project selector.

`GET /api/projects/:id/repositories` returns sorted project-relative repository
paths (`.` for the root). Discovery walks directories, skips `skipDirs`, and
does not follow symlink directories. A `.git` directory or worktree file marks
a repository, including nested checkouts. Discovery runs when refreshing the
workspace; no repository records or clone metadata are stored.

The right header shows a Git repository dropdown only when this list contains
more than one entry. One repository is used automatically; zero entries leave
the Git panes empty. Switching projects clears the selection and selects the
first repository. Switching repositories keeps the project and task active,
clears old Git results, and loads the selected repository's Changed and Commits.
Late responses from another project or repository cannot replace those lists.

Changes, log, and commit endpoints accept `repo`, bounded to the project folder
and checked for a Git root. Changed and commit file paths remain relative to
the whole project. Diff and image endpoints find the nearest enclosing Git
root from that path, so already-open file tabs retain their repository when
the header selection changes. Tree and task working directories use the full
project folder.

Validation: server tests cover discovery, worktrees, scoped changes/history,
commit file paths, diffs, and rejected paths; browser tests cover header
placement, multiple repositories within one project, selection, and hiding
the selector for single repositories and multiple independent projects.
