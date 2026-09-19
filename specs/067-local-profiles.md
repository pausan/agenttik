# Local profiles

Profiles are local to a data directory on this computer. Settings → Profiles adds,
renames, reorders, and removes profiles. Up/down buttons save the shared order
for Settings and profile pickers, including the built-in profile. The built-in
profile, initially named Default, holds existing data and cannot be removed. Names are trimmed, unique ignoring
case, and limited to 80 bytes.
Renaming changes the display name only: the stable ID, data, running work, and
open profile remain unchanged. A pencil button beside each name opens the inline
editor.

The sidebar footer shows a profile picker immediately before Shortcuts only when
more than one profile exists. The command palette offers `Switch to profile: <name>`
for every other profile and refreshes the list when opened. Switching reloads the window. Other profiles keep
running tasks and schedules. `Ctrl+Alt+P` selects the next profile and
`Ctrl+Alt+Shift+P` the previous one, wrapping in saved order. Both are editable
in Shortcuts and ignore key repeats. Unsaved file edits use the existing browser unload
warning. A profile in the window URL scopes each API request, stream, image, and
saved tab; windows can use different profiles concurrently.

Each added profile has its own SQLite database, attachments, search index,
settings, model favourites and visibility, projects, tasks, history, and schedules.
Provider connections, named subscriptions, their default choice, and System
subscription display names are shared across the instance. API keys and discovered
catalogs are shared too; API conversation files remain profile-local
([078](078-api-providers.md)). Models and automatic-action model choices stay
profile-local. The same folder
can be registered independently in multiple profiles. Working files are shared:
profiles do not create copies of the project directory. Browser preferences are
scoped too, including Light/Dark/System mode and file-tree expansion. Default
retains its existing browser keys; added profiles use separate keys.

Existing profile subscriptions are imported into the instance database at startup.
IDs in tasks, schedules, queues, favourites, visibility and action models are
remapped; duplicate aliases receive a numeric suffix. Managed login directories
move under the instance's `accounts/profiles/<uuid>/` so deleting a profile cannot
remove a shared login. Existing instance defaults take precedence.

## Moving projects

Project Options in the right sidebar offers **Move to profile** when another
profile exists. Choose a destination and click **Move project**. The current
window stays in its profile; both profiles' open windows refresh their lists.
The working folder stays in place. The profile-owned orchestrator cannot move.

The move carries the project prompt, tasks (including archived tasks), messages,
turn usage, outcomes, schedules and their run history. Session UUIDs stay stable;
integer IDs and their references are remapped to avoid destination collisions.
Images referenced by project/task prompts, messages or schedules and direct API
conversation files are copied into the destination. Source managed files remain
because another task may reference them. Browser preferences and drafts do not move.

Moved tasks and schedules retain their shared subscription IDs and provider
threads. Direct API conversations get a fresh file ID pointing to copied history.
All moved schedules arrive paused.

Running turns, queued prompts (including retries), and running schedule jobs block
a move. Unsaved file edits in the initiating window also block it. Successful moves
close source terminals. A destination already using the folder returns a conflict;
projects are never merged. Missing files or insert failures leave both databases
unchanged and remove files created by the failed attempt.

`POST /api/profiles/:destination/projects/:projectID/move?profile=:source` returns
`{id, profile_id}`. The profile manager excludes routed requests and profile removal;
the source runner excludes turn starts and schedule ticks during transfer. An
attached SQLite transaction transfers rows and deletes the source project after
file copies succeed. Both databases use WAL: ordinary errors roll back both, but
SQLite does not guarantee cross-database atomicity on a machine crash.

## Storage and routing

- Existing Default data stays at `<data-dir>/agenttik.db` without migration.
- `profiles.json` holds stable UUIDs and display names in display order, written
  by atomic rename.
- Added profiles live under `<data-dir>/profiles/<uuid>/`, each with a database,
  instance lock, runner, and clocks. They reopen at startup, so schedules resume
  even when another profile is being viewed.
- `GET/POST /api/profiles` lists/creates; `PATCH /api/profiles/:id` renames;
  `DELETE /api/profiles/:id` removes. `PUT /api/profiles/order` accepts
  `{ids: [...]}` containing every current profile exactly once; missing, duplicate, or
  unknown IDs leave the catalog unchanged.
- API URLs carry `?profile=<uuid>`; missing means Default and unknown IDs return
  404. Native desktop, network-server, and remote-connection controls remain at
  instance scope. Profile management is shared across the instance.
- A profile's loopback listener and lock support `--data-dir <profile-dir> --api`
  and orchestrator calls without crossing into Default.

Removal requires an in-app confirmation. It stops the profile's clocks and turns,
closes its server and database, and removes its managed directory. Project working
files remain. The final built-in profile keeps the app usable.

Private mode puts the catalog and every profile under its temporary data root.
Private tabs, drafts, and app preferences stay in memory and are lost on reload;
they never enter the normal browser storage. The footer identifies private mode.

Validation: Go tests cover isolation, same-folder projects, settings, persistence,
CLI discovery, local model visibility/favourites, shared subscriptions, friendly
System names, invalid names, deletion, and shutdown.
Move tests cover ID collisions, history and image copies, return moves, blocked
work, duplicate folders, failed-copy rollback, and moves in either direction.
Browser tests cover the picker, palette switching, appearance, tree expansion,
renaming, reordering, and removal; private tests cover concurrent instances and
SIGTERM cleanup. Startup remains under the one-second budget.
