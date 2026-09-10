# Single instance

One agenttik runs per data directory. Launching a second one raises the window
of the first and exits, rather than opening a rival copy over the same
database.

## The lock

`<data-dir>/agenttik.lock` carries an advisory `flock(LOCK_EX|LOCK_NB)`, taken
in `run()` before `store.Open` and held for the life of the process
(`app/internal/single`). The kernel releases it when the file closes, so a
crash or a `SIGKILL` leaves nothing to clean up by hand.

The data directory is the key, not the port. That is what two instances would
actually corrupt: one SQLite file, opened twice. `--data-dir` therefore still
starts a genuine second copy, which is how the e2e suite runs its servers in
parallel ([005-testing.md](005-testing.md)).

Taking the lock before the database matters. `ResetRunningSessions` clears
whatever the table says is running, because no turn survives a restart — and a
second launch used to run it against the *first* instance's live turns. It
would mark them `interrupted` while their CLI child was still going, and the
runner and the database disagreed from there on. In web mode the port clash hid
this unless the second launch passed a different `--addr`; the desktop shell
listens on an ephemeral port, so every extra launch did it.

## The handoff

The lock file's contents are the address its owner serves on, written by
`Publish` as soon as the listener is up — the ephemeral port in desktop mode,
`--addr` in web mode. A second launch reads it, posts to `/api/foreground`, and
prints what happened:

| Answer | What the second launch prints |
|--------|-------------------------------|
| `{"raised": true}` | `agenttik is already running; brought its window to the front` |
| `{"raised": false}`, or no answer | `agenttik is already running on http://<addr>` |
| lock held, no address yet | `agenttik is already running` |

It exits 0 either way. Launching the app twice is not an error; it is a request
to look at the copy already open.

`Acquire` truncates the file the moment it takes the lock, so the address a
hard-killed instance left behind cannot be reported as live by a launch that
arrives before the new owner publishes its own.

## Raising the window

`Server.OnForeground` holds the hook, registered during wiring before anything
serves, and `/api/foreground` reports whether calling it found a window. Only
the desktop shell sets one, from the Wails context that `OnStartup` hands over;
a mutex guards that context because the server answers requests before the
window exists. Web mode leaves the hook nil, which is what makes `raised` false
there — there is no window to raise, only a URL to print.

The hook calls `runtime.WindowShow` then `runtime.WindowUnminimise`. On Linux
Wails maps those to `gtk_widget_show`, which covers a window that was hidden,
and `gtk_window_present`, which is the raise and the focus. Both queue onto the
GTK main loop, so an HTTP goroutine may call them. Under Wayland a compositor
may still refuse a raise it did not see the user ask for.

Wails ships its own `SingleInstanceLock`, over dbus, and it is not used: it
runs inside `wails.Run`, which is long after the database is open and reset, it
does nothing in a web-only build, and it exits 1 on the path that worked.

## Status

Implemented and checked end to end.

Web mode: a second launch on the same data directory printed the running URL
and exited 0, whether or not it was given the same `--addr`, and the first
instance kept serving. With a fake `claude` holding a turn open, the session
stayed `running` and its turn stayed `running` across the second launch. The
same probe against a `HEAD` binary built in a throwaway worktree flipped the
session to `idle` and the turn to `interrupted` while the child still ran,
which is the regression the ordering fixes. `SIGTERM` emptied the lock file;
`SIGKILL` left the address behind but the flock free, and the next start took
it.

Desktop mode, headless on Xvfb under xfwm4: the window opened and was the
active window, `xdotool windowminimize` left it `IsUnMapped` with no active
window, and a second launch printed the raise line, exited 0 and left the
window `IsViewable` and active again. Process and toplevel-window counts were
unchanged across the second launch, so nothing was left behind.
