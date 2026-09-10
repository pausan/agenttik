# Editable project path

## Outcome

A project's folder can be changed after it is added, from the same Options
pane its name is renamed in. This is for when the folder on disk moves — the
existing project row is repointed rather than deleting the project and
re-adding it, so its tasks and history stay attached.

The folder is set two ways: typed into the field, or browsed for. The button
beside the field opens the same `FolderPicker` the Add project dialog uses,
already listing the folder the project points at now, so a folder that moved
next door is a click or two rather than a walk from home. `Select` applies
it, `Cancel` leaves the project alone.

Either way the path is validated as a new project's is: expanded from `~`,
made absolute, and required to already exist as a directory. A rejected path
leaves the stored one untouched. On success the Tree pane and the project's
stats follow the new folder at once; so does every turn started afterwards,
since the working directory is read from the database at turn start rather
than cached on the session.

Two projects cannot share a folder — `projects.path` is unique, and one
folder with two owners would give its Tree, its Changed pane and its turns
two of everything. Picking a folder another project holds answers `409` with
"another project already uses that folder", for a repoint and for Add
project alike.

## Choices

**One field, one PATCH, applied independently.** `PATCH /api/projects/:id`
already renamed a project with `{name}`. It now also accepts `path`, and
applies whichever of the two arrives — a rename does not require sending the
path back, and a move does not require the name. `resolveProjectPath` is the
validation `createProject` already did, pulled out so a moved folder is
checked exactly as strictly as a new one.

**The picker is reused, not rebuilt.** `FolderPicker.open()` takes an
optional folder to start in; with no argument it re-lists the one in view, so
the Add project dialog is unchanged. The picker returns a trailing slash —
that is what makes the next keystroke filter *inside* the folder — so
`updateProjectPath` strips it, otherwise re-picking the current folder would
read as a change.

**The constraint is translated once, in the store.** `pathTaken` matches
`SQLITE_CONSTRAINT_UNIQUE` and returns `ErrPathInUse`, which the error
handler maps to `409`. Doing it at the store covers create and update from
one place, and keeps SQLite's own wording out of a toast.

**The pane refetches on success, the same way switching projects does.**
`updateProjectPath` calls `refreshInspector()` after a successful move, so
the Tree shows the new folder's files at once instead of waiting for a
filesystem event. This does not reach the live `fsnotify` watcher of 019:
that watcher is keyed by project id and starts once per open window, so a
window already watching the old folder keeps watching it until its event
stream reopens — switching projects, or a reload. Explicit actions — opening
the Tree, saving a file, starting a turn — read the path fresh regardless.

## Validation

`go build ./...`, `go vet ./...` and `gofmt -l` are clean. `make ui` builds
and the 33 web unit tests pass. `go test ./...` reproduces only the two
failures already on record in the index (`TestDoneReachesProjectTopic`,
`TestReorderSessionsDrivesProjectOrder`), unrelated to this change.

Browser-checked headlessly against a built web server, on scratch folders
`projA` and `projB` under one root, with no unexpected console or page
errors:

| Done in the UI | Result |
|---|---|
| Add project through its own picker | unchanged — lands on `projA` |
| open the Options pane | Folder field reads `projA`, Browse button present |
| click Browse | picker opens listing `projA`, breadcrumbs live |
| `..`, into `projB`, `Select` | stored path, field and Tree all move to `projB` |
| Browse, navigate, `Cancel` | stored path unchanged |
| type a path into the field | still applies on blur |
| Browse to a folder another project holds | toasts "another project already uses that folder"; stored path unchanged |

`curl` covers the same rejection on `POST /api/projects` and on
`PATCH /api/projects/:id`: both answer `409` with that message and write
nothing.
