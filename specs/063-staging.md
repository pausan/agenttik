# Staging and commits

The task's Changed pane starts with an open, collapsible **Staged** section.
Staged and unstaged files have separate counts; partially staged files appear
in both lists. File rows still open the file and offer Show in tree.

An up arrow from a base stages a file. A down arrow to a base unstages it.
Each section also offers an all-files button. Unstaging preserves working
files, including before the first commit. Renames include both index paths.

A compact commit message textarea and Commit button sit below Staged.
Focusing the textarea opens a modal with a roughly 72-column monospace editor
and the button; small screens cap it to the viewport. Closing preserves the
draft. Switching project or repository clears it. Commit needs staged files
and a nonblank message. Git errors keep the draft and display inline.
Successful commits clear the message and refresh changes and history.

`GET /api/projects/:id/changes?repo=…` returns project-relative paths, status,
`staged`, and `unstaged`. NUL-delimited porcelain preserves unusual filenames.
`POST /api/projects/:id/stage` and `/unstage` accept `{path}` (project-relative)
or `{}` for all applicable changes. `POST /api/projects/:id/commit` accepts
`{message}` and commits the index with normal Git hooks (a two-minute timeout allows hooks to finish). All three accept the
selected `repo` query parameter. Paths are matched against Git status and
passed as literal pathspecs. Operations never push.
