# Local profiles

Profiles are local to a data directory on this computer. Settings → General → Profiles adds,
renames, and removes profiles. The built-in profile, initially named Default,
holds existing data and cannot be removed. Names are trimmed, unique ignoring
case, and limited to 80 bytes.
Renaming changes the display name only: the stable ID, data, running work, and
open profile remain unchanged. A pencil button beside each name opens the inline
editor.

The sidebar footer shows a profile picker immediately before Shortcuts only when
more than one profile exists. The command palette offers `Switch to profile: <name>`
for every other profile and refreshes the list when opened. Switching reloads the window. Other profiles keep
running tasks and schedules. Unsaved file edits use the existing browser unload
warning. A profile in the window URL scopes each API request, stream, image, and
saved tab; windows can use different profiles concurrently.

Each added profile has its own SQLite database, provider accounts, attachments,
search index, settings, model favourites and visibility, projects, tasks, history,
and schedules. Subscription lists and default subscription choices are isolated. API keys,
model catalogs and API conversation files are separate per profile too
([078](078-api-providers.md)). The same folder
can be registered independently in multiple profiles. Working files are shared:
profiles do not create copies of the project directory. Browser preferences are
scoped too, including Light/Dark/System mode and file-tree expansion. Default
retains its existing browser keys; added profiles use separate keys.

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

Credentials and provider settings stay in their profile. Tasks and schedules reset
to the system subscription; tasks previously using a named subscription start a
new provider conversation on their next prompt while retaining visible history.
System-subscription conversations keep their thread, and direct API conversations
get a fresh file ID pointing to the copied history. All moved schedules arrive
paused, so the user can configure destination providers before resuming them.

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
- `profiles.json` holds stable UUIDs and display names, written by atomic rename.
- Added profiles live under `<data-dir>/profiles/<uuid>/`, each with a database,
  instance lock, runner, and clocks. They reopen at startup, so schedules resume
  even when another profile is being viewed.
- `GET/POST /api/profiles` lists/creates; `PATCH /api/profiles/:id` renames;
  `DELETE /api/profiles/:id` removes.
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
CLI discovery, favourites, subscriptions, invalid names, deletion, and shutdown.
Move tests cover ID collisions, history and image copies, return moves, blocked
work, duplicate folders, failed-copy rollback, and moves in either direction.
Browser tests cover the picker, palette switching, appearance, tree expansion,
renaming, and removal; private tests cover concurrent instances and
SIGTERM cleanup. Startup remains under the one-second budget.
