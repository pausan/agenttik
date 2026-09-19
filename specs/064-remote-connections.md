# Remote connections

`agenttik --remote host:port` opens a desktop client for another agenttik
server. HTTP(S) origins are also accepted, including bracketed IPv6 hosts.
A bare address uses HTTP. Paths, query strings, fragments and embedded
credentials are rejected. Use HTTPS when a TLS proxy fronts the server.

Remote startup checks the server before opening a window. It does not open
SQLite, take the local instance lock, start providers, or run schedules.
`--web --remote host:port` (also the fallback in web-only builds) prints a
random loopback URL for a browser client. Closing that process ends its
client session. `--remote` cannot be combined with `--init`, `--api`, or
`--body`. `--addr` and `--data-dir` are unused in remote mode.

The command palette offers **Connect to remote server**. Its dialog accepts
an address and checks it without leaving the current instance. It previews
the server name and build version; Connect then checks again and reloads
the desktop window against the remote server;
in a normal browser, it navigates to the checked remote origin. Local tasks
keep running when a local desktop window switches to a remote. Restart
without `--remote` to return to the local instance. Save or send drafts
before switching; unsaved file edits block the connection until saved or
discarded. Restored desktop tabs and drafts are keyed by remote
origin, so matching task IDs on different servers do not share drafts.

## Finding instances from the CLI

`agenttik --find-remotes` scans active, non-loopback private IPv4 interfaces
and exits without opening a window, database, or instance lock. Each interface
address is expanded to /8 and intersected with private address space:
`10.0.0.0/8`, `172.16.0.0/12`, or `192.168.0.0/16`. Duplicate ranges are scanned
once. Public, link-local, loopback, and IPv6 addresses are excluded.

Discovery probes HTTP port 7717 by default. `--addr :PORT` selects another
port; its host is ignored. Each match prints its URL, quoted name, and quoted
version to stdout, separated by tabs, as soon as it is found. Progress and
the match count go to stderr. No matches is a successful result; no eligible
interfaces is an error. Ctrl+C cancels the scan and exits nonzero, preserving
matches already printed. The flag cannot be combined with other action modes
or positional arguments. `--data-dir` is unused.

The scan uses 128 workers, a 500 ms deadline per address, and bounded memory.
A silent /8 can take about 18 hours; slow servers may be missed. Probes bypass
HTTP proxies, do not follow redirects, and validate the same public
`/api/version` signature used by remote connections. Password-protected
instances are discoverable. Only the chosen HTTP port is scanned; arbitrary
ports, HTTPS listeners, and loopback-only desktop backends are not discovered.

## Server names

Each server database receives a persistent generated name, `agenttik-` plus
eight random hexadecimal digits. Settings → Server allows renaming in both
desktop and web mode. Names are trimmed and must contain at least three
Unicode code points. Invalid changes leave the saved name intact. Valid
inputs use normal styling; editing clears previous validation feedback.
`PUT /api/server/name` accepts `{"name":"My workstation"}` behind the normal
access gate. Identity is independent of listener and authentication settings.
Profiles served by one process share its server identity.

The sidebar displays the current server name. Remote CLI startup reports the
name and version and uses the name in the window title. Names are public
discovery metadata, available before login. Older servers without a name
remain supported; the connection preview uses their address.

## Discovery and authentication

`GET /api/version` returns JSON:

```json
{"application":"agenttik","version":"1.2.3","name":"agenttik-a1b2c3d4"}
```

The version is the same build value as `--version` (`dev` in unversioned
builds). This endpoint is public even when the exposed listener requires
login, and uses `Cache-Control: no-store`. Other endpoints remain protected.

The check has a five-second deadline and a 4 KiB response limit. It requires
HTTP 200, valid JSON, the exact application name, and a nonempty version.
It never follows discovery redirects. Old servers without this endpoint
must be upgraded. A reverse proxy must also allow discovery without login.
This signature detects mistaken URLs; it is not cryptographic proof of
identity or an API compatibility guarantee. HTTPS uses normal certificate
verification.

After discovery, the remote server serves its existing login form when
needed: agenttik's built-in protection asks for a password and, when 2FA is enabled, an authenticator
code, with no username. The client does not save passwords. Remote desktop
and loopback clients keep a separate in-memory cookie jar per connection,
strip local cookies and authorization headers, and keep login redirects
within the checked origin. Login failure, expiry and revocation use the
same server behavior as direct browser access. SSE flushes immediately.

Native desktop clients translate HTTP 303 navigation responses into a small
HTML page that navigates within the window. WebKit custom URI schemes do not
follow HTTP redirects. Cookies are saved before navigation, so a successful
password/code login opens the authenticated UI. Browser clients and fetch
requests retain HTTP redirect behavior. The desktop setting survives switching
servers through the palette; redirects outside the checked origin are refused.
The native Linux regression test submits the real login form under Xvfb.

`POST /api/remote/connect` accepts `{"address":"host:port"}` and returns
`{"url":"…","version":"…"}` only after discovery succeeds. The desktop
proxy handles this locally; browser servers return a navigation URL. The
handler requires JSON and rejects cross-origin requests. Failed checks
leave the active proxy intact. A successful check stages the new target;
the proxy changes when the window navigates, after saving its old state.

`POST /api/remote/check` accepts the same body and returns the same metadata
without staging or changing the active target. Both responses include
`name` when supplied by the server.

Tests cover name generation, persistence, validation, preview isolation,
parsing, signature failures and redirects, public discovery
behind the authentication gate, cookie isolation, switching failures, and
password/TOTP login and revocation through the remote proxy.
