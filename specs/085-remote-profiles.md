# Remote profiles

A window can open another machine's profiles without leaving its own
instance. Local tasks and schedules keep running, and `agenttik --remote`
([064](064-remote-connections.md)) still works as before.

## Adding a machine

Settings → Profiles → **Remote machines** takes a `host:port` or HTTP(S)
URL. The server checks `/api/version`, and needs the `id` field there. Servers
too old to send it are refused, and so is this instance's own address. The
machine is saved by that ID. Adding an address that reaches a machine already
saved adds it to that entry, which keeps its sign-in and its other addresses.

Each saved machine lists its name, the address in use, how many more it
knows (all of them on hover), its version and its profiles. A
refresh button checks the machine and reloads its profile list. Remove forgets
the machine and its sign-in; its data stays on that machine. Profiles are
added, renamed and removed on their own machine.

## Searching for machines

Settings → Profiles → **Search for machines** scans the same ranges, in the
same order, as `--find-remotes`, using port 7717 by default. The port can be changed before
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

The scan probes up to 2048 addresses at once with 500 ms deadlines, as
described in [064](064-remote-connections.md), and keeps only its network
cursor, interrupted addresses and matches in memory. A paused scan has no
open sockets or goroutines. The UI polls progress only while scanning; shutdown
cancels the scan. General discovery and finding a saved machine share the
same network scope and never run together.

## The picker

Once a machine is saved, the profile picker groups profiles under machine
names: the flat view of all of them first ([100](100-flat-view.md)), then
this instance's name, then each remote's. Remote profiles use the
person icon with a small Wi‑Fi badge, a red cross while the machine is down
(see Connecting). The footer button shows that icon when
the window is on a remote profile, and its title reads
`Profile: <machine> › <profile>`. The command palette offers
`Switch to profile: <machine> › <profile>`, with a Wi‑Fi-off icon for a
machine that is down. `Ctrl+Alt+P` and its reverse move
through the flat view, local profiles, then each machine's, wrapping.

Busy dots pulse beside working local and connected remote profiles in the
menu. The footer dot shows work in other local profiles and any connected
remote profile, including the selected remote profile. The current local
profile's task status stays in the workspace.

Keyboard shortcuts use the client's shared bindings, including the
Enter-send/enqueue choice. Ctrl/Cmd defaults follow the client's OS rather
than the remote host; see [015](015-settings-shortcuts.md).

## Connecting

Startup reads the saved list and contacts no remote. A machine is contacted
only when you switch to one of its profiles, check it in Settings, or open a
window already on it. Before the window navigates,
`POST /api/remotes/:id/connect` finds an address that answers with the same
machine ID, fetches its profile list and learns the machine's addresses. A
profile that no longer exists falls back to the remote's first one.

After a successful connection, one server worker per machine refreshes its
profiles and busy state every three seconds, with a three-second deadline.
It finds the machine as above before fetching profiles with the saved sign-in.
This continues while the window is hidden or showing a local profile. Failed
requests, expired sign-ins and a different machine at the address clear the
working dots. Removing the machine or
shutting down cancels its worker. Busy state stays in memory and is never
restored from disk. The desktop tray shows remote-only work with a Wi-Fi
badge; local work always takes priority ([060](060-desktop-tray.md)).

A machine is **down** once a request to it, or the search for its address,
fails or runs out of time; a request whose caller left does not count. It is
up again as soon as anything reaches it, even with a sign-in prompt.
`GET /api/remotes` marks it `down`. While it is down, background callers
leave it alone: the worker above and the flat view ([100](100-flat-view.md))
try it once every 60 s, the first to ask after the pause. Opening one of its
profiles always tries it at once, and shows the dialog below if it still
does not answer.

The window's URL is `/?remote=<machine-id>&profile=<remote-profile-id>`. The
UI comes from this instance; every `/api/` request carries `remote=` and is
forwarded by `routeRemote`. These stay on this instance: `/api/remotes`,
`/api/remote/`, `/api/profiles`, `/api/updates`, `/api/desktop` and
`/api/foreground`. Everything else goes to the remote, including Server
settings, the version shown in About and the sidebar's machine name. Projects
move between local profiles only, so Project Options hides the move on a
remote.

The UI and the remote API can differ in version. Keep both machines updated.

## Addresses

A machine can have a local network address and a public one, such as its
quick tunnel ([084](084-quick-tunnel.md)) or a forwarded port. Each saved
machine keeps up to eight: the one in use first, then the others from the
most recently reached. Addresses join the list when they are added, found on
the network, or reported by the machine.

`GET /api/server/addresses` (behind the lock) reports where a machine can be
reached: each listener as `http://ip:port`, once per non-loopback,
non-link-local IPv4 interface address when it listens on all of them, then
the tunnel's URL once it is ready. After every successful connection, this
instance asks for it and appends new ones. Older servers answer 404 and keep
what is known.

Finding the machine is a quick check of the known addresses, in order: local
network addresses (private, loopback and link-local IPs, names without a dot
or under `.local`, `.lan` or `.home.arpa`) before public ones, then the most
recently reached. The first starts at once; the next starts when the last
fails or after 300 ms. The best address that answers with the right machine
ID wins. Once one answers, better ones still pending get one more second.
At home this is one request; away, up to about 1.5 s. A new winner becomes the
address in use and the most recent. With none, it is `moved` when another
agenttik answered at one of them, else `unreachable`.

This check runs on connect and on every activity tick, so a machine moves to
its local address when it is back home and to its public one when it is
away. A forwarded request that fails to connect runs it too and, when it
picks another address, is sent once more there.

## Sign-in

If the remote asks for sign-in, the dialog asks for its password, and for a
code when its form has one. This instance posts the remote's own login form
and keeps the session cookie in `remotes.json`, which is mode 0600 and never
sent to the browser. A forwarded request that comes back 401 carries
`X-Agenttik-Remote-Auth: required`, and the UI opens the sign-in dialog rather
than reloading. The sign-in also makes the remote trust this machine's key,
so later sessions, after a restart, an idle timeout or a new address, start
by themselves. See [094](094-trusted-machines.md).

## When the machine is gone

A window that opens on a remote checks the machine before loading anything
else. If no known address reaches it, the dialog offers **Retry**, **Find on network** and
**Use a local profile**. Switching from the picker shows the same dialog, with
Cancel in place of the last.

**Find on network** runs the `--find-remotes` scan on the server and stops at the first match on the machine ID. The
address is then added and used, and the connection tried again. The scan goes from the
most local range to the widest: every active interface's /24, then its /16,
then its private /12 or /8 (192.168 stops at /16). A range skips addresses an
earlier one covered, so nothing is probed twice. The status lists each range
with the addresses it adds; the dialog derives the current range from the
probe count and shows `Searching 192.168.x.x (132 of 65,280)` with a bar,
`Range 2 of 3 · next 10.x.x.x`, the overall count and **Stop searching**. One
scan runs at a time, and shutdown stops it.

The scan probes plain HTTP on the saved address's port when that address was
`http://` with an explicit port. Any other address, such as a public tunnel
(`https://…trycloudflare.com`, or `http://` with no port), says nothing about
the port in the machine's network, so the scan uses agenttik's default, 7717.

## Storage and API

`<data-dir>/remotes.json` holds `[{id, name, address, addresses, version,
profiles, cookies}]` and is written by atomic rename. Entries saved before
`addresses` existed start with their `address`. Private mode keeps it under the
temporary root.

| Method | Path | Does |
|--------|------|------|
| GET | `/api/remotes` | Saved machines with `addresses`, without cookies; profiles include live `busy` state for connected machines |
| POST | `/api/remotes` | `{address}`: check and save; answers like connect |
| POST/GET/DELETE | `/api/remotes/discovery` | Start or continue (`{port?, restart?}`), read `{running, complete, port, probed, total, machines, error}`, pause |
| DELETE | `/api/remotes/:id` | Forget the machine |
| POST | `/api/remotes/:id/connect` | `{status: ok\|signin\|unreachable\|moved, error, code_required, remote, busy}` |
| POST | `/api/remotes/:id/login` | `{password, code}`; 401 with the remote's reason |
| POST/GET/DELETE | `/api/remotes/:id/find` | Start, read `{running, probed, total, networks: [{network, hosts}], found, error}`, stop |

Forwarded requests pass only `Accept`, `Cache-Control`, `Content-Type`, the
conditional and range headers and `Last-Event-ID`. They never pass the
browser's cookies, origin or update token. Event streams are flushed chunk by
chunk. A closed window is noticed at the remote's next heartbeat; shutdown
cancels the stream at once.

Validation: Go tests cover forwarding, local-only paths, persistence, refusing
this instance's own address, password and code sign-in, cookie privacy, a
moved machine, listener addresses, address order, the quick check's stagger
and grace, learning addresses and moving to another when one stops, finding it on a loopback network, and streaming. A browser
test adds a second real instance, switches to and from its profile, and opens
it after it stopped. Discovery tests cover pause, resume without skipped or
repeated addresses, restart, deduplication, filtering, connection-triggered
pause and shutdown. A browser test exercises the controls and connects to
two real machines with deterministic discovery results.
Activity tests cover initial and refreshed busy state, overlapping remote
work, expired sign-ins, reconnects, removal and shutdown. `remote_down_test.go`
covers when a machine counts as down and that it is tried once per pause. A browser test checks
the footer and menu dots through two real instances, from both a local and a
remote profile.
