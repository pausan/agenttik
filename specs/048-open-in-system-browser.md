# Opening a project or a file on the desktop

A right click on a project row in the sidebar, and on any row of the Tree,
offers **Open in system browser**: the folder or the file is handed to the
machine agenttik runs on, which opens it the way a double click in a file
manager would — a folder in the file browser, a file in whatever is registered
for its type.

A right click that misses every Tree row aims at the project folder, so the
pane's empty space opens the project itself. Nothing here asks first: opening
something changes nothing.

The project row's menu wraps the row alone, not the block around it. The jobs
and tasks below it are rows of their own, and a right click on one of those is
not aimed at the project.

## The endpoint

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/projects/:id/open` | `{path}` — empty is the project folder itself |

The path is resolved through `resolveEntry` ([046](046-tree-file-actions.md)),
so it carries the same rules the Tree's other three do: relative to the
project, no traversal out of it, nothing under `.git`. The answer is that
file's `{path, dir}`.

Then one command per platform, the same three Wails' own `BrowserOpenURL`
runs: `xdg-open`, `open`, or `rundll32.exe url.dll,FileProtocolHandler`.

## Choices

**The server opens it, not the window.** The desktop shell could have called
Wails' runtime directly, but then a `--web` launch and a browser on another
machine ([043](043-exposed-server.md)) would have had no way to do this at
all — and they are pointed at the one desktop the projects are actually on,
which is the desktop to open them on. One code path serves all three shells.

**Started, not waited for.** The opener is a launcher: it exits as soon as the
real application has been asked. The request answers on `Start`, so a handler
that fails afterwards is not reported — what is reported is the path not
existing, and the opener not being installed, which are the two failures worth
a message. The child is reaped in a goroutine so it does not sit as a zombie
for the life of the app.

**It opens nothing new.** The exposed server has no login, and this endpoint
launches a program on the machine running it. That is not a fresh exposure:
the same server already reads, writes and deletes any file in a project, and
runs agent turns that can do anything at all. It is bounded to the project
folder for the same reason those are.
