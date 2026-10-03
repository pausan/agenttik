# Remote profiles

A window can open another machine's profiles without leaving its own
instance. Local tasks and schedules keep running, and `agenttik --remote`
([064](064-remote-connections.md)) still works as before.

## Adding a machine

Settings → Profiles → **Remote machines** takes a `host:port` or HTTP(S)
URL. The server checks `/api/version`, and needs the `id` field there. Servers
too old to send it are refused, and so is this instance's own address. The
machine is saved by that ID. Adding an address that reaches a machine already
saved moves that entry to the new address and keeps its sign-in.

Each saved machine lists its name, address, version and its profiles. A
refresh button checks the machine and reloads its profile list. Remove forgets
the machine and its sign-in; its data stays on that machine. Profiles are
added, renamed and removed on their own machine.

## Searching for machines

Settings → Profiles → **Search for machines** scans the same private ranges
as `--find-remotes`, using port 7717 by default. The port can be changed before
starting or restarting. Progress and matches appear as the scan runs. Results
exclude this instance and servers without a machine ID, and list each ID once.
Discovery does not save machines or fetch their profiles.

**Pause search** cancels active probes. **Continue search** keeps the results
and continues with unfinished addresses, including interrupted probes.
**Restart search** clears discovery results and progress, reads the active
networks again, and starts on the chosen port. Saved machines stay saved.
Connecting to a result pauses first, saves the machine, and opens its first
profile (or asks for sign-in). Other matches remain available for connecting
to more machines. Adding, opening or checking a saved remote also pauses
discovery.
Closing Settings pauses it; reopening Settings, including after switching to
a remote profile, restores progress and results. Search state lasts until
this instance shuts down. One search is shared by this instance's windows.

The scan uses 128 workers with 500 ms probe deadlines and keeps only its
network cursor, interrupted addresses and matches in memory. A paused scan
has no active workers. The UI polls progress only while scanning; shutdown
cancels the scan. General discovery and finding a saved machine share the
same network scope and never run together.

## The picker

Once a machine is saved, the profile picker groups profiles under machine
names: this instance's name first, then each remote's. Remote profiles use the
person icon with a small Wi‑Fi badge. The footer button shows that icon when
the window is on a remote profile, and its title reads
`Profile: <machine> › <profile>`. The command palette offers
`Switch to profile: <machine> › <profile>`. `Ctrl+Alt+P` and its reverse move
through local profiles, then each machine's, wrapping.

Busy dots cover local profiles only. A remote is not polled.

## Connecting

Startup reads the saved list and contacts no remote. A machine is contacted
only when you switch to one of its profiles, check it in Settings, or open a
window already on it. Before the window navigates,
`POST /api/remotes/:id/connect` checks that the saved address answers with
the same machine ID, and fetches its profile list. A profile that no longer
exists falls back to the remote's first one.

The window's URL is `/?remote=<machine-id>&profile=<remote-profile-id>`. The
UI comes from this instance; every `/api/` request carries `remote=` and is
forwarded by `routeRemote`. These stay on this instance: `/api/remotes`,
`/api/remote/`, `/api/profiles`, `/api/updates`, `/api/desktop` and
`/api/foreground`. Everything else goes to the remote, including Server
settings, the version shown in About and the sidebar's machine name. Projects
move between local profiles only, so Project Options hides the move on a
remote.

The UI and the remote API can differ in version. Keep both machines updated.

## Sign-in

If the remote asks for sign-in, the dialog asks for its password, and for a
code when its form has one. This instance posts the remote's own login form
and keeps the session cookie in `remotes.json`, which is mode 0600 and never
sent to the browser. A forwarded request that comes back 401 carries
`X-Agenttik-Remote-Auth: required`, and the UI opens the sign-in dialog rather
than reloading. Sessions end when the remote restarts or after its idle
timeout.

## When the machine is gone

A window that opens on a remote checks the machine before loading anything
else. If the machine does not answer, or another machine answers at its
address, the dialog offers **Retry**, **Find on network** and
**Use a local profile**. Switching from the picker shows the same dialog, with
Cancel in place of the last.

**Find on network** runs the `--find-remotes` scan on the server. It covers
the private /8, /12 and /16 ranges of the active interfaces, on the port of
the saved address, and stops at the first match on the machine ID. The
address is then saved and the connection tried again. The dialog shows how
many addresses were probed and has **Stop searching**. One scan runs at a
time, and shutdown stops it. HTTPS addresses cannot be scanned.

## Storage and API

`<data-dir>/remotes.json` holds `[{id, name, address, version, profiles,
cookies}]` and is written by atomic rename. Private mode keeps it under the
temporary root.

| Method | Path | Does |
|--------|------|------|
| GET | `/api/remotes` | Saved machines, without cookies |
| POST | `/api/remotes` | `{address}`: check and save; answers like connect |
| POST/GET/DELETE | `/api/remotes/discovery` | Start or continue (`{port?, restart?}`), read `{running, complete, port, probed, total, machines, error}`, pause |
| DELETE | `/api/remotes/:id` | Forget the machine |
| POST | `/api/remotes/:id/connect` | `{status: ok\|signin\|unreachable\|moved, error, code_required, remote, busy}` |
| POST | `/api/remotes/:id/login` | `{password, code}`; 401 with the remote's reason |
| POST/GET/DELETE | `/api/remotes/:id/find` | Start, read `{running, probed, total, found, error}`, stop |

Forwarded requests pass only `Accept`, `Cache-Control`, `Content-Type`, the
conditional and range headers and `Last-Event-ID`. They never pass the
browser's cookies, origin or update token. Event streams are flushed chunk by
chunk. A closed window is noticed at the remote's next heartbeat; shutdown
cancels the stream at once.

Validation: Go tests cover forwarding, local-only paths, persistence, refusing
this instance's own address, password and code sign-in, cookie privacy, a
moved machine, finding it on a loopback network, and streaming. A browser
test adds a second real instance, switches to and from its profile, and opens
it after it stopped. Discovery tests cover pause, resume without skipped or
repeated addresses, restart, deduplication, filtering, connection-triggered
pause and shutdown. A browser test exercises the controls and connects to
two real machines with deterministic discovery results.
