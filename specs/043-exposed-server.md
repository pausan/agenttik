# Exposed server

## Outcome

Settings gained a sixth section, **Server**: whether this desktop window's
server also answers a browser — this machine or another on the network — in
addition to the window itself, and where.

The window's own connection is untouched either way: it always talks to a
random loopback port through its own reverse proxy, exactly as before. Server
is a second, independent listener proxying to that same backend, so turning
it on, off, or pointing it elsewhere never interrupts the window.

Off by default. The host is one of three: **Everybody (0.0.0.0)**,
**localhost (127.0.0.1)**, or **Other**, which reveals a field for a typed
IP. The port starts on the app's own default — 7717, or whatever `--addr`
was given — and is otherwise free text. The choice is saved and re-applied
the next time the window opens, the same way a saved project or a starred
model is.

Not offered on a web launch (`--web`): the process is already the exposed
server, on the address `--addr` gave it, so there is nothing here to turn on.
`GET /api/server` answers `{"available": false}` there and the pane says so
instead of drawing the controls.

There is no login anywhere in agenttik. Settings says as much next to the
switch, and again as a standing warning whenever the host is not loopback:
picking Everybody or a LAN address means anyone who can reach that address
and port has the same access this window does.

## Choices

**Two listeners, one backend, one `*fiber.App`.** `app/internal/netserver`
is a small manager that reverse-proxies a chosen `host:port` to the loopback
address the desktop window already listens on — the same technique
`app/cmd/agenttik/desktop.go` already used for the window's own view, right
down to `FlushInterval = -1` so SSE still streams. It is a plain
`net/http.Server`, not a second `fiber.Listener` call on the shared app: Fiber
shuts every listener registered to one `*fasthttp.Server` down together, and
the window's own listener must survive a browser listener being switched off.

**Starting replaces only after the new one is up.** `Manager.Start` binds the
new address first and only shuts the previous `*http.Server` down once the
new one is already serving. A bad host or a taken port therefore leaves
whatever was working exactly as it was, both live and in `Status()` — see the
validation run below for the case of a rejected port.

**The saved setting is the source of truth; live status is a separate
read.** `store.ServerConfig{Enabled, Host, Port}` is one row
(`server_config`, id fixed at 1). `GetServerConfig` returns the zero value
until ever saved — blank host, 0 port — rather than a migration hard-coding
7717 a second time; `Server.withDefaults` fills those blanks with whatever
`runDesktop` was given as the app's own default, from `config.Default()` or
`--addr`. `GET /api/server` merges that saved-or-defaulted setting with
`netserver.Status()`, so a setting that fails to re-apply at startup — its
port taken by something else since the last run — still reads as `enabled`
with `listening: false` and the bind error, rather than silently reverting to
off.

**`PUT /api/server` applies before it saves.** Turning the server on with a
fresh address and pointing a running one at a new address are the same
request: validate the host as an IP and the port as 1–65535, attempt the
bind, and only write to SQLite once that succeeds. A rejected `PUT` changes
neither the live listener nor the saved setting.

**The UI applies each control at the moment it means something.** The switch
and the Everybody/localhost radio choices call `PUT` the instant they change,
like General's Enter/Enqueue pair. Other is different: picking it only
reveals the field, because a fresh "Other" and whatever well-known host was
saved before are otherwise indistinguishable — it applies on blur, once a
host has actually been typed, the same as the project path field. The port
field validates its range client-side before ever calling `PUT`, so a typo
never reaches the network layer at all.

## Validation

`go build ./...`, `go vet ./...`, `gofmt -l` and the desktop-tagged build are
clean. `make ui` and the web unit tests (33) pass. `go test ./...` reproduces
only the two failures already on record in the index
(`TestDoneReachesProjectTopic`, `TestReorderSessionsDrivesProjectOrder`),
unrelated to this change.

The listener swap needs no window, so it was driven directly against the real
`store`, `netserver` and `server` packages — the same wiring
`runDesktop` does, minus Wails: a fresh launch reports the compiled-in
default (`available: true`, `enabled: false`, `127.0.0.1:7717`,
`listening: false`); enabling on a free port binds it and a request through
that port reaches the real API; pointing it at a port something else already
holds answers `400` and leaves the previous good listener answering exactly
as before; stopping the manager and rebuilding the server against the same
database — a stand-in for relaunching the app — re-binds the saved address
on its own, with no request needed; disabling stops the exposed port while
the loopback address the window itself would be using keeps answering
throughout.

Browser-checked headlessly with `/api/server` mocked so the controls render
regardless of launch mode: starts unchecked, localhost selected, port 7717,
no status line while disabled; enabling sends `{enabled: true, host:
"127.0.0.1", port: 7717}` and draws "Listening on 127.0.0.1:7717" with no
warning; picking Everybody sends `host: "0.0.0.0"` and draws the network-wide
warning; picking Other alone sends nothing until a typed address is blurred,
which then sends that host and names it in the warning; a port outside
1–65535 is rejected before any request and the field snaps back to the saved
value; disabling sends `enabled: false` and clears the status line. No page
errors; the one console error is `fail`'s own logging of the port rejection,
same as every other rejected edit in the app.
