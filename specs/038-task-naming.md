# Task naming

The app calls each user-visible conversation a **task**: sidebar rows, project
lists, archived lists, shortcuts, the launcher, and action labels use that
name. A task row's hover popup starts with its sidebar title (or `Untitled
task`) and then shows up to five lines of its opening prompt.

Client actions and the row component use task names. The HTTP API, database,
and provider protocol retain `session` names and fields because they are
