# Exposed server

Settings gained a sixth section, **Server**: whether this desktop window's
server also answers a browser — this machine or another on the network — in
addition to the window itself, where, and whether it asks who is knocking.

The window's own connection is untouched by any of it: it always talks to a
random loopback port through its own reverse proxy, exactly as before. Server
is a second, independent listener proxying to that same backend, so turning
it on, off, pointing it elsewhere or putting a login on it never interrupts
the window.

Off by default. The host is one of three: **Everybody (0.0.0.0)**,
**localhost (127.0.0.1)**, or **Other**, which reveals a field for a typed
IP. The port starts on the app's own default — 7717, or whatever `--addr`
was given — and is otherwise free text. The choice is saved and re-applied
the next time the window opens, the same way a saved project or a starred
model is.

Not offered on a web launch (`--web`): the process is already the exposed
server, on the address `--addr` gave it, so there is nothing here to turn on.
`GET /api/server` answers `{"available": false}` there and the pane says so
instead of drawing the controls. A `--web` launch therefore has no login
either — it is the one way to put agenttik on a network without one.

Once it is up, the pane says what to open: `Listening on` and the address as
a link, which hands that page to the machine's own browser, with an icon
beside it that copies the same URL.

## The lock

A second switch, **Ask for a password and a code**, puts a login in front of
that listener. It is off by default, and the pane carries a standing warning
whenever the host is not loopback and the switch is off: anyone who can reach
that address and port has the same access this window does.

Turned on, a browser reaching the address gets a login form asking for a
password and the six digits an authenticator app is showing. The window is
never asked. It reaches the backend down its own loopback proxy, which the
gate is not in front of, so a forgotten password is an inconvenience rather
than a lock-out — Settings is still right there to set a new one.

The authenticator seed is shown as selectable text to copy, as a QR to scan,
and as a field to type into: a seed that already exists somewhere can be
pasted in rather than paired afresh, and the button beside it rolls a random
one for a seed that has been seen by the wrong person. Changing either the
seed or the password signs every browser out.

## Choices

**Two listeners, one backend, one `*fiber.App`.** `app/internal/netserver`
is a small manager that reverse-proxies a chosen `host:port` to the loopback
address the desktop window already listens on — the same technique
`app/cmd/agenttik/desktop.go` already used for the window's own view, right
down to `FlushInterval = -1` so SSE still streams. It is a plain
`net/http.Server`, not a second `fiber.Listener` call on the shared app: Fiber
shuts every listener registered to one `*fasthttp.Server` down together, and
the window's own listener must survive a browser listener being switched off.

**The lock is a wrapper on that listener, not middleware on the app.**
`Manager.Use` takes a `func(http.Handler) http.Handler` and puts it in front
of the proxy. Fiber middleware would have had to tell a request that arrived
through the exposed listener from one the window made, and the two are
indistinguishable by the time they reach the shared backend — both are plain
loopback requests. Wrapping the one listener needs no such marker, and cannot
be bypassed by forging one: it is the only thing in front of that socket.

**Starting replaces only after the new one is up.** `Manager.Start` binds the
new address first and only shuts the previous `*http.Server` down once the
new one is already serving. A bad host or a taken port therefore leaves
whatever was working exactly as it was, both live and in `Status()`.

**The saved setting is the source of truth; live status is a separate
read.** `store.ServerConfig` is one row (`server_config`, id fixed at 1).
`GetServerConfig` returns the zero value until ever saved — blank host, 0
port — rather than a migration hard-coding 7717 a second time;
`Server.withDefaults` fills those blanks with whatever `runDesktop` was given
as the app's own default, from `config.Default()` or `--addr`.
`GET /api/server` merges that saved-or-defaulted setting with
`netserver.Status()`, so a setting that fails to re-apply at startup — its
port taken by something else since the last run — still reads as `enabled`
with `listening: false` and the bind error, rather than silently reverting to
off.

**Two writers on one row, so neither can clear the other.** The address and
the lock share `server_config` but are set at different moments for different
reasons, so `SetServerConfig` and `SetServerAuth` each write only their own
columns through an `ON CONFLICT DO UPDATE`. Moving the server to another port
is not a reason to ask for the password again, and setting a password is not
a reason to rebind. `PUT /api/server` reads the row and changes the address
on it rather than building a fresh struct, for the same reason on the way
out: answering with a partial one reported the lock as gone every time the
port moved.

**`PUT /api/server` applies before it saves.** Turning the server on with a
fresh address and pointing a running one at a new address are the same
request: validate the host as an IP and the port as 1–65535, attempt the
bind, and only write to SQLite once that succeeds. A rejected `PUT` changes
neither the live listener nor the saved setting.

**The link is the address that can be opened, not always the one that was
bound.** Everybody asks for 0.0.0.0, and a dual-stack machine reports the
listener it got back as `[::]`; neither is somewhere a browser can go. Both
are drawn as `http://localhost:<port>`, which reaches that same listener from
the machine it runs on — the standing warning above is what says it answers
the network as well. Any other host is shown exactly as it bound.

**A browser keeps the click; the window hands it to Wails.** The line is a
plain `target="_blank"` anchor, which is the whole story in a browser: a web
launch, or another machine reading this same pane over the exposed server,
opens a tab of its own. The desktop window's webview opens nothing at all for
one, so there the click comes off the anchor and goes to `BrowserOpenURL`
instead. Wails' runtime is injected into the window's own page and into no
other, so whether it is there is both the way to do this and the way to tell
the two shells apart — no flag has to be plumbed through for it.

**The UI applies each control at the moment it means something.** The switch
and the Everybody/localhost radio choices call `PUT` the instant they change,
like General's Enter/Enqueue pair. Other is different: picking it only
reveals the field, because a fresh "Other" and whatever well-known host was
saved before are otherwise indistinguishable — it applies on blur, once a
host has actually been typed. The port field validates its range client-side
before ever calling `PUT`. The lock switch applies in both directions immediately.
Enabling it generates a random TOTP seed if none exists. Without a saved
password, browser access is blocked and the pane explains how to unlock it.
Passwords are entered once and saved with Save or Enter.
The current six-digit code appears beside the seed and QR, with a copy button.
While the Server pane is visible, `GET /api/server/auth/code` returns the server-clock code and
milliseconds until the next 30-second boundary, when the pane refreshes it.
The endpoint uses `Cache-Control: no-store` and the same access gate as settings.
Polling stops when the pane or dialog closes; failed refreshes clear the code.

## How the lock is built

`app/internal/netauth` is the whole of it.

**TOTP is RFC 6238 on the standard library.** HMAC-SHA1 over a 30-second
counter, six digits, only the current and immediately previous steps accepted.
No other digest or length is offered, because no widely used authenticator app offers one in
a QR, so offering them here would only be a way to pair with nothing. The
generator is checked against the RFC's own published vectors. `rsc.io/qr`
draws the `otpauth://` URI; it is the only dependency the feature added.

**A code is spent once.** The gate tracks the two accepted counters independently
for the seed. An unused previous code works even after a current-code login;
older and future codes are rejected. Changing the seed permits its new codes
immediately. Reusing a code shows a message to wait for the next code.

**Passwords are bcrypt, and guessing is throttled.** Each stored bcrypt string
contains its random salt, cost, and hash; plaintext passwords are never stored
or returned by the API. bcrypt's default cost
puts about a tenth of a second under every attempt on its own; five wrong
ones from an address lock that address out for a quarter of an hour, counted
from the connection's own remote address rather than a forwarding header,
which would otherwise be a way to get somebody else locked out. Both halves
of the login are checked even when the first already failed, so a wrong
password and a wrong code cost the same and take the same time to say so.

**Sessions live in memory only.** A login mints a random 32-byte token, kept
in a map against a 12-hour idle deadline and handed over as an `HttpOnly`,
`SameSite=Lax` cookie. There is no signing key on disk to steal, closing
agenttik ends every browser session with it, and `Gate.Revoke` — which any
change of password or seed calls — ends them all at once. `SameSite=Lax` is
also what keeps another site from making the API do anything with that
cookie, since no CSRF token is minted anywhere.

**A half-set lock opens for nobody.** Enabled with no password, or no seed,
refuses every login rather than letting everybody through. This is a supported
setup state: the switch stays enabled while a password is being added.

**A browser gets a page; everything else gets a status.** An unauthenticated
navigation is answered with a self-contained login form and nothing else —
not the UI bundle, not an asset, not a page title. The public
`GET /api/version` discovery endpoint is the sole exception; it identifies
the application and build version before login (see [064](064-remote-connections.md)). Any request that is not a
navigation, which is every fetch the UI makes and the SSE stream, gets a
`401` with a JSON body instead, because handing those an HTML login page
where they expect data only breaks them strangely. The UI treats any `401` as
the lock, since agenttik's own API never sends one, and reloads the page into
the form.

**The form remembers where you were going, and only where.** A deep link
asked for while signed out comes back in a hidden field and is redirected to
after a good login. Anything that is not a path on this same server is
dropped for `/`, so the field cannot be handed a target somewhere else.

**Signing out is a URL, not a button.** `/__auth/logout` drops this browser's
session and returns to the form. Nothing in the UI links to it: the pane that
would hold the link is the one place the lock is administered from, and it is
usually being read in the window, which has no session to end.

## What this is not

The listener speaks plain HTTP. The password and the code cross the network
in the clear, and so does everything the app does afterwards. On a home or
office LAN that is the same exposure the unlocked server already had, with a
lock added in front of it; across the open internet it is not enough on its
own, and the address wants a TLS-terminating proxy or a tunnel in front of
it. Nothing in the pane sets one up.
