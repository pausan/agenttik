# Task naming

The app calls each user-visible conversation a **task**. Sidebar rows, project
lists, the archive, shortcuts, the launcher and every action label use that
word, and an unnamed one reads `Untitled task`.

The HTTP API, the database and the provider protocols keep `session` names and
fields: they are a wire and storage contract, and renaming them would migrate a
schema and break every stored client value to change a word nobody reads. So
`sessions.title`, `POST /api/sessions` and `claude --session-id` stay as they
are, and only what reaches the screen says task.

These specs follow the same split: **task** for the thing on screen, **session**
for the row, the route or the CLI flag.

A task row's hover popup starts with its title, or `Untitled task`, and then
shows up to five lines of the prompt it was opened with — see
[004](004-ui.md#tabs). How the title itself is chosen is
[020](020-task-titles.md).
