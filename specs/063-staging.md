# Staging and commits

The task's Changed pane has an inline commit composer above an open, collapsible
**Staged** section. Its empty state uses compact vertical padding.
Staged and unstaged files have separate counts; partially staged files appear
in both lists. File rows still open the file and offer Show in tree.

A plus button stages a file. A minus button unstages it.
Both lists have an undo-arrow button on each row, with a tooltip, to revert
the file after confirmation (discarding both staged and unstaged changes).
Each section also offers an all-files button. Unstaging preserves working
files, including before the first commit. Renames include both index paths.

A vertically resizable commit message textarea sits above Staged, with the
Commit button below it, aligned to the right. Editing stays inline. Switching project or
repository clears the draft. Commit needs staged files
and a nonblank message. Git errors keep the draft and display inline.
A four-point star left of Commit generates or replaces the message from only
staged changes. Both buttons have tooltips. Generation uses the task's provider
and account (the first available provider and its default account outside a task),
with its lightweight model in an isolated, read-only request outside the repository.
It never commits. The composer and staging controls are disabled while generating;
errors preserve the draft, and switching project or repository discards late answers.

`POST /api/projects/:id/commit-message?repo=…` accepts optional `provider` and
`account_id`, and returns `{message}`. It reads the index with external diffs and
text conversion disabled, rejects empty diffs and diffs over 128 KiB, and allows
one minute for generation. No task or conversation is created.

Successful commits clear the message and refresh changes and history.

`GET /api/projects/:id/changes?repo=…` returns project-relative paths, status,
`staged`, and `unstaged`. NUL-delimited porcelain preserves unusual filenames.
`POST /api/projects/:id/stage` and `/unstage` accept `{path}` (project-relative)
or `{}` for all applicable changes. `POST /api/projects/:id/commit` accepts
`{message}` and commits the index with normal Git hooks (a two-minute timeout allows hooks to finish). All three accept the
selected `repo` query parameter. Paths are matched against Git status and
passed as literal pathspecs. Operations never push.
