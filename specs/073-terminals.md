# Terminals

A terminal is an interactive shell running in a project's folder, drawn in a
tab of its own beside the conversation. It is the same folder the agent works
in, so a `git diff` or a `make test` next to the transcript needs no second
window and no second way of finding the checkout.

## What is on screen

The button is at the right-hand end of the line of chord hints under the prompt
box: **Terminal**. Each press opens another shell in the current project's
folder, as a tab in that project's strip named after the shell and numbered —
`fish 1`, `fish 2`. The tab's `×` closes the terminal, which ends the shell and
everything it started. A shell that ends itself — `exit`, or Ctrl+D — takes its
tab with it.

The button is not drawn on a phone, and is not drawn over a terminal or a file,
which have no prompt bar. It needs a task open, since that is what puts the
prompt bar on screen.

Terminals belong to the project, not to a conversation:

- Selecting another project hides them with the rest of that project's strip,
  and leaves every shell running.
- Coming back shows them again, in the order they were opened.
- Changing which task is in front changes nothing about them. A project has one
  context slot ([008](008-project-workspaces.md)) and a terminal is an
  independent tab beside it, like a file.

They are not saved with the rest of the strip. A shell does not outlive the app
that started it, so quitting ends every terminal and a fresh launch has none —
which is what closing a terminal window does anywhere else. A *reloaded* window
is different: the server is still up and its shells with it, so the tabs come
back. That falls out of where the list is kept rather than being a special case:
the UI asks the server which terminals a project has whenever that project is
selected, and reconciles its tabs with the answer.

Deleting a project ends its terminals, beside stopping its tasks.

## The shell

`$SHELL`, or the first of `/bin/bash` and `/bin/sh` that exists; on Windows the
first of `pwsh.exe`, `powershell.exe` and `cmd.exe` on `PATH`. A terminal that
is not the shell you configured is a terminal with the wrong aliases, prompt and
history.

No login flag. Started on a tty with no command to run, every common shell is
already interactive and reads its interactive startup file; asking for a login
shell as well would re-read the profile on every tab.

The environment is the app's own, with `TERM=xterm-256color` and
`COLORTERM=truecolor` replacing whatever it inherited. Without `TERM` anything
using curses refuses to start.

## How it travels

Output comes back over SSE and keystrokes go out over POST. A WebSocket is the
obvious shape for a terminal and is not available: the desktop window is served
through a Wails `http.Handler`, which can stream a response but cannot be
hijacked into a two-way connection. SSE is the one live channel that works in
the window, in a browser and through the exposed server
([043](043-exposed-server.md)) alike.

| Route | What it does |
|-------|--------------|
| `GET /api/projects/:id/terminals` | the project's shells, oldest first |
| `POST /api/projects/:id/terminals` | start one, optionally at a given size |
| `DELETE /api/terminals/:id` | end one |
| `POST /api/terminals/:id/input` | keystrokes, as raw bytes |
| `POST /api/terminals/:id/resize` | a changed pane size |
| `GET /api/stream/terminals?ids=…` | every open terminal's output |

One stream carries every terminal, each frame naming its own, for the same
reason `/api/stream` carries every session ([004](004-ui.md#live-updates)): a
browser holds only a handful of connections to one origin. It is a *second*
stream rather than part of that one because the two carry different things — a
`find /` in a terminal writes faster than anything a turn publishes, and a
watcher that falls behind is dropped. Sharing one connection would let a busy
shell cost the window its session events.

### Resuming

Each terminal keeps the last 256KB of its output, and each watcher says how many
bytes of it that view already has: `?ids=<id>:<from>`. Every frame carries the
count it brings the watcher to, which the next request sends back.

This is what makes the shared stream safe. The stream is reopened whenever the
set of terminals on it changes, so without a resume point, opening a second
terminal would redraw the first one's screen on top of the screen already
showing it. A watcher with nothing, or one so far behind that what it missed has
been trimmed away, is handed the scrollback as a whole screen instead, flagged
`reset` so the view clears before writing it. A watcher that falls too far
behind has its stream closed, which is not a loss: reconnecting resumes it.

The replay is a byte slice cut at an arbitrary point, so a very old escape
sequence can be halved by the cut. An emulator discards a partial sequence,
which is why the cut is allowed to be that careless.

### Keystrokes

One request per terminal is in flight at a time. Two would be free to arrive in
either order, and a shell given its input out of order is a shell given
different input. Anything typed while one is in flight joins the next, so
holding a key down costs one request rather than thirty.

The joined keystrokes go out as one flat byte array, never as a `Blob`. Linux
WebKit segfaults inside Wails' URI-scheme handler when it reads a `Blob` body,
which took the whole app down on the first character typed — the same crash
pasted images avoid ([056](056-pasted-images.md)).

A resize is sent as soon as the pane has been measured — until the shell is
told, it is drawing its prompt for a window it is not in — and every resize
after that waits for the dragging to stop, since a full-screen program redraws
itself on each one.

## Choices

**A pseudo-terminal, not a pipe.** `app/internal/terminals` runs the shell on a
pty through `github.com/aymanbagabas/go-pty`, which is ConPTY on Windows and
`openpty` everywhere else. A shell on a pipe is not interactive, draws no
prompt, and refuses to run anything that wants a tty — which is most of what a
terminal is for.

**The parent lets go of the slave end.** Once the shell has started, this
process closes its copy of the pty's slave end. Holding it keeps the master
readable forever, so a shell that exited on its own would never reach the end of
its output and its tab would sit on a dead screen waiting for more.

**Nothing reaches SQLite.** A shell is a running thing, and a row saying one
used to exist would only ever describe a process that had already gone. The
manager holds them for as long as the process does, and `Shutdown` ends them.

**Closing a terminal ends its process group.** Closing the pty hangs the shell
up, and a `SIGKILL` two seconds later takes anything that ignored the hangup. A
build left running in a closed terminal ends with it rather than carrying on
unattached — the same rule `app/internal/process` applies to a stopped turn.

**A closed terminal is not on the reopen stack.** `Ctrl+Shift+T`
([007](007-task-closing.md)) brings back a view of something still there.
Closing a terminal ends the shell, so there is nothing to bring back.

**The emulator is loaded when one is first opened.** xterm.js is a couple of
hundred kilobytes, in a chunk of its own, so a launch that never opens a
terminal never pays for it — see [052](052-launch-budget.md).

**The button sits with the chord hints.** That line is already the quiet one
under the prompt, and the right-hand end of it is empty. A terminal is not a
prompt action, so putting it in the Send menu ([012](012-task-queue.md)) would
say that it was.

## Validation

`app/internal/terminals` is tested against real shells: a command runs in the
project's folder, a late watcher is handed the screen, one stream carries
several terminals, a resize reaches the shell, a shell that exits reaches its
watcher, and the scrollback stays capped. `api_terminals_test.go` covers the
routes and the 404 a terminal that has gone answers with.

`web/src/terminals.test.js` covers the two careful parts of the client:
keystrokes keep their order across an in-flight request, report-mode bytes are
not widened into text, keystrokes travel as bytes rather than as a `Blob`, and
a reopened stream resumes each terminal where its view got to.

`make test-desktop-terminal` types through the real module inside Wails/WebKit,
where a `Blob` body is a native crash rather than a failed assertion
([005](005-testing.md)).

`e2e/tests/terminals.spec.js` drives the whole thing in a browser — a command
running in the project's folder, closing a terminal, switching project and back
to find the same two in the same order still holding what was typed, a second
task leaving them alone, and `exit` taking a tab with it. The fixture pins
`SHELL` to `/bin/sh` so the tab names and the screen read the same on every
machine.
