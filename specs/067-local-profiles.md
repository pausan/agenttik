# Local profiles

Profiles are local to a data directory on this computer. Settings → Profiles adds
and removes profiles. The built-in Default profile holds existing data and cannot
be removed. Names are trimmed, unique ignoring case, and limited to 80 bytes.

The sidebar footer shows a profile picker immediately before Shortcuts only when
more than one profile exists. Switching reloads the window. Other profiles keep
running tasks and schedules. Unsaved file edits use the existing browser unload
warning. A profile in the window URL scopes each API request, stream, image, and
saved tab; windows can use different profiles concurrently.

Each added profile has its own SQLite database, provider accounts, attachments,
search index, settings, projects, tasks, history, and schedules. The same folder
can be registered independently in multiple profiles. Working files are shared:
profiles do not create copies of the project directory. Browser preferences are
scoped too, except the existing browser-wide Light/Dark/System setting.

## Storage and routing

- Existing Default data stays at `<data-dir>/agenttik.db` without migration.
- `profiles.json` holds stable UUIDs and display names, written by atomic rename.
- Added profiles live under `<data-dir>/profiles/<uuid>/`, each with a database,
  instance lock, runner, and clocks. They reopen at startup, so schedules resume
  even when another profile is being viewed.
- `GET/POST /api/profiles` lists/creates; `DELETE /api/profiles/:id` removes.
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
CLI discovery, invalid names, deletion, and shutdown. Browser tests cover the
picker, switching and removal; private tests cover concurrent instances and
SIGTERM cleanup. Startup remains under the one-second budget.
