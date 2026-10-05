# Staging and commits

The task's Changed pane has an inline commit composer above an open, collapsible
**Staged** section. Its empty state uses compact vertical padding.
Staged and unstaged files have separate counts; partially staged files appear
in both lists. File rows still open the file and offer Show in tree.
Both lists reflect Git status independently of rendered diffs. Whitespace-only
and line-ending-only changes remain listed, as do staged changes whose working
copy matches HEAD again (the file then appears in both lists).

A plus button stages a file. A minus button unstages it.
Both lists have an undo-arrow button on each row, with a tooltip, to revert
the file after confirmation (discarding both staged and unstaged changes).
Each section also offers an all-files button. Unstaging preserves working
files, including before the first commit. Renames include both index paths.

A vertically resizable commit message textarea sits above Staged, with the
current branch below it on the left and the generate-message and Commit icon
buttons on the right. Long branch names truncate and show the full name in a
tooltip. The branch is available before the first commit too. Commit uses a Git
commit symbol and a text label. Editing stays inline. Each project and repository
keeps its own draft in `S.commitDrafts`, keyed by `[projectID, repository]`.
Switching panes, tabs, projects or repositories preserves the text, including
whitespace. Drafts are saved with open tabs in browser storage, scoped to the
profile and server, and restored on reload. Private windows keep them in memory
only. Typing is saved after a 300 ms debounce; leaving the page flushes it.
The textarea waits for repository discovery before accepting edits. Only manual
clearing, successful commits, or successful message generation remove or replace
text. Commit needs staged files and a nonblank message. Git errors keep the draft
and display inline.
A four-point star left of Commit generates or replaces the message from only
staged changes. Both buttons have tooltips. Generation uses the configurable automatic-action model and subscription
(see [069](069-automatic-action-models.md)), defaulting to a lightweight model in an isolated, read-only request outside the repository.
It never commits. The composer and staging controls are disabled while generating;
errors preserve the draft. Switching project or repository, or leaving the
Changed pane, discards late generation answers.

`POST /api/projects/:id/commit-message?repo=…` accepts `{}` and returns `{message}`. It reads the index with external diffs and
text conversion disabled, rejects empty diffs and diffs over 128 KiB, and allows
one minute for generation. No task or conversation is created.

Successful commits clear the submitted repository's message even if the user has
navigated elsewhere; edits made after submitting remain for the next commit.
Changes and history refresh when that repository is still selected.
Staging, unstaging and committing stop their busy indicators as soon as the Git
request answers. Changes and history refresh afterward, including on failure;
slow or failed remote reads do not keep the controls busy. Failed commits keep
the message for retry.

`GET /api/projects/:id/changes?repo=…` returns project-relative paths, status,
`staged`, and `unstaged`. NUL-delimited porcelain preserves unusual filenames.
`POST /api/projects/:id/stage` and `/unstage` accept `{path}` (project-relative)
or `{}` for all applicable changes. `POST /api/projects/:id/commit` accepts
`{message}` and commits the index with normal Git hooks (a two-minute timeout allows hooks to finish). All three accept the
selected `repo` query parameter. Paths are matched against Git status and
passed as literal pathspecs. Operations never push.
