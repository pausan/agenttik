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
an address, checks it, and displays failures without leaving the current
instance. Success reloads the desktop window against the remote server;
in a normal browser, it navigates to the checked remote origin. Local tasks
keep running when a local desktop window switches to a remote. Restart
without `--remote` to return to the local instance. Save or send drafts
before switching; unsaved file edits block the connection until saved or
discarded. Restored desktop tabs and drafts are keyed by remote
origin, so matching task IDs on different servers do not share drafts.

## Discovery and authentication

`GET /api/version` returns JSON:

```json
{"application":"agenttik","version":"1.2.3"}
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
needed: agenttik's built-in protection asks for a password and authenticator
code, with no username. The client does not save passwords. Remote desktop
and loopback clients keep a separate in-memory cookie jar per connection,
strip local cookies and authorization headers, and keep login redirects
within the checked origin. Login failure, expiry and revocation use the
same server behavior as direct browser access. SSE flushes immediately.

`POST /api/remote/connect` accepts `{"address":"host:port"}` and returns
`{"url":"…","version":"…"}` only after discovery succeeds. The desktop
proxy handles this locally; browser servers return a navigation URL. The
handler requires JSON and rejects cross-origin requests. Failed checks
leave the active proxy intact. A successful check stages the new target;
the proxy changes when the window navigates, after saving its old state.

Tests cover parsing, signature failures and redirects, public discovery
behind the authentication gate, cookie isolation, switching failures, and
password/TOTP login and revocation through the remote proxy.
