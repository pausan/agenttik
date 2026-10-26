# Flat view

The flat view shows every profile at once: this machine's and those of each
saved remote ([085](085-remote-profiles.md)). It saves switching between
profiles, most of all on a phone.

## The picker

Once there is more than one profile in total, the picker, the command palette
and `Ctrl+Alt+P` start with **Flat view** (a layers icon). It opens
`/?profile=all`. Every other part of the window works as in a profile of its
own.

Each project row names where the project lives before its folder:
`Laptop › Work · /path`. The machine is named only when a remote is saved;
with local profiles alone, the row names just the profile. The same folder in
two profiles, or on two machines, is two rows. **Move to profile** is
hidden: projects move from a window on their own profile.

## Order and hidden projects

The flat view has its own project order and its own hidden projects, apart
from every profile's. A drag can mix projects from any profile and machine;
no profile's order changes. Hiding a project here takes it out of the flat
view only; its profile still shows it. The hidden menu names each project's
place and restores it at the index it was hidden from. A project hidden in its
own profile is not in the flat view at all, and comes back from that profile.

Until the first drag, projects follow their own profile's order, this
machine's Default first, then its other profiles, then each remote's.
Projects the flat view has not placed yet, such as a new one, go on top, or at
the bottom when Settings puts new items there. The orchestrator stays first. The footer's busy dot stays off, since all work is on
screen already.

The model picker, favourites, model visibility, Settings' subscriptions and
the Screen tab ([096](096-screen.md)) follow the profile of the project in
front. Moving to a project on another profile or machine reads its models and
its screen again. Screen is offered on a remote's projects, named
`Screen · <machine>`, and on this machine's only from another device. New
projects are added on this machine's Default. Everything else in Settings is this machine's Default,
including the orchestrator settings.

## The orchestrator

Each profile keeps its own orchestrator. The flat view lists only this
machine's Default orchestrator. When there is more than one profile or any
saved remote, its per-turn runtime context
([054](054-orchestrator-project.md)) tells it to add `profile=all` to every
API path. It then reads and acts on everything in the flat view, and uses
each project's source to choose between projects with the same name. The
saved prompt does not change. Other profiles' orchestrators stay within their
profile.

## Server

`routeFlat` serves every `/api/` request with `profile=all` and no `remote`.
Only the instance's own server has it; added profiles have no remotes. A
source is one profile: this machine's (forwarded in process through the app's
own handler, as `routeProfile` does) or a remote's (sent with the saved
session, as `routeRemote` does).

- **IDs.** Project and job IDs become `id << 16 | code`, where `code` is a
  16-bit FNV hash of `remote/profile`. A collision takes the next free code, in
  key order, so codes survive restarts and sources coming and going. A code
  that names no current source answers 404. Task IDs are UUIDs and stay as
  they are. Terminal IDs stay as they are too.
- **Rewriting.** In every JSON answer, `project_id` and `schedule_id` are
  encoded. A top-level `id` is encoded on project and job routes, and on
  `schedules` inside projects. Projects gain
  `source: {id, machine, profile, remote}`. Task IDs seen in any answer or
  event are remembered with their source. Terminal IDs are remembered from
  `/api/projects/:id/terminals`.
- **Routing.** `/api/projects/:id/…` and `/api/schedules/:id/…` decode the
  path. `/api/sessions/:id/…` uses the remembered source, or asks every source
  at once on a miss. `/api/terminals/:id` and `/api/stream/terminals` use the
  terminal's source; the stream's `ids` are `id:bytes drawn`, and the first
  one's source carries them all, since the window shows one terminal at a
  time. Otherwise a `project_id` in the query or the JSON body
  picks the source. Anything else goes to this machine's Default.
- **Merged lists.** `GET /api/projects` (also `?hidden`, `?archived`), and
  `GET /api/sessions`, `GET /api/schedules` without `project_id`, and
  `GET /api/approvals` ask every source at once. Tasks and jobs are re-sorted
  as `store.ListSessions` and `store.ListSchedules` sort them, then paged.
  Only other sources' orchestrators are dropped.
- **Own order and hidden set.** This machine's Default database keeps
  `flat_projects`: a row per project, keyed by source (`remote/profile`) and
  the project's ID there, with its position and when it was hidden.
  `GET /api/projects` sorts the merged list by it and drops the hidden rows;
  `?hidden=true` answers those rows as `{id, name, source}`, most recently
  hidden first. `POST /api/projects/order` rewrites the visible positions and
  drops visible rows it does not name. A `PATCH /api/projects/:id` that only
  sets `hidden` stays here: hiding keeps the project's visible index, as
  a profile does ([061](061-hidden-projects.md)), and restoring inserts it there.
  Both send `projects_changed` on this machine's Default, which every flat
  window hears.
- **Stream.** `/api/stream` opens one stream per source, with the topics split
  by owner (a task not yet placed goes to every source, where unknown IDs never
  fire), and writes them as one SSE stream with its own heartbeat. A source
  that drops is reopened after 1 s, doubling to 30 s. When it comes back, a
  `projects_changed` event makes the window read the lists again.
- **Unreachable machines.** A list waits at most 4 s for a source. A remote
  that fails is skipped for 20 s. Only this machine's Default failing fails a
  list. A remote that wants sign-in answers 503, telling the user to sign in
  from Settings → Profiles, rather than 401: the flat view cannot open one
  machine's sign-in dialog for another.
- **Where the browser is.** A request for this machine's own profiles runs
  in process, from loopback, so it carries the browser's address in
  `X-Forwarded-For` for the screen's `same_machine`.
- **Images.** The window uploads pasted images to this machine. Before a
  request goes to another source, every `/api/attachments/<hash>` it mentions
  is uploaded there once. Names are content hashes. `GET /api/attachments/…`
  tries each source in order.

## Limits

Smart search ([057](057-smart-search.md)) and analytics read this machine's
Default only.

## Validation

Go tests (`flat_test.go`) cover merged lists across local profiles and a
remote, same-folder projects in two profiles, routing of project, task,
favourite requests, a terminal stream to its profile, the screen seeing a
browser on another device, a flat order that mixes profiles and leaves theirs
alone, hiding and restoring in the flat view only, where unplaced projects
go, an unknown source, task lookup after a restart,
the merged stream carrying a remote's event, lists that do not wait on a
stopped remote, image copies, stable distinct codes and the orchestrator's
flat context. `e2e/tests/flat-view.spec.js` opens the flat view from the
picker, checks the source labels, runs a task on a remote's project and finds
it on that machine, checks the labels in the phone drawer, drags a
project between two profiles' projects and hides and restores one, and runs a
terminal on another profile and on a remote, with Screen offered only on the
remote's project.
`profiles.spec.js` checks the picker order and the shortcut cycle.
