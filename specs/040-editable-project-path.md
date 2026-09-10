# Editable project path

## Outcome

A project's folder can now be changed after it is added, from the same
Options pane the name is renamed in. This is for when the folder on disk
moves — the existing project row is repointed rather than deleting the
project and re-adding it, so its tasks and history stay attached.

The new path is validated the same way a new project's is: expanded from
`~`, made absolute, and required to already exist as a directory. A rejected
path leaves the stored one untouched. On success the Tree pane and the
project's stats immediately follow the new folder; so does every turn
started afterward, since the working directory is read from the database at
turn start, not cached on the session.

## Choices

**One field, one PATCH, applied independently.** `PATCH /api/projects/:id`
already renamed a project with `{name}`. It now also accepts `path`, and
applies whichever of the two arrives — a rename does not require sending the
path back, and a move does not require the name. `resolveProjectPath` is the
validation `createProject` already did for a new project's path, pulled out
so both handlers check a moved folder exactly as strictly as a new one.

**The pane refetches on success, the same way switching projects does.**
`updateProjectPath` calls `refreshInspector()` after a successful move, so
the Tree pane shows the new folder's files at once instead of waiting for
the next filesystem event. This does not reach the live `fsnotify` watcher
described in 019: that watcher is keyed by project id and starts once per
open window, so a window already watching the old folder keeps watching it
until its event stream reopens (switching projects, or a reload). Explicit
actions — opening the Tree, saving a file, starting a turn — all read the
path fresh regardless.

## Validation

`go build ./...`, `go vet ./...` and `gofmt -l` are clean. `make ui` builds
and the 33 web unit tests pass. `go test ./...` reproduces only the two
failures already on record in the index
(`TestDoneReachesProjectTopic`, `TestReorderSessionsDrivesProjectOrder`),
unrelated to this change.

Browser-checked headlessly against a built web server: added a project on a
scratch folder A (Folder field read back `A`, Tree read back `a.txt`), edited
Folder to a scratch folder B and blurred — the server's stored path became
`B`, the field showed `B`, and the Tree pane switched to `b.txt` with no
reload. Editing Folder to a path that does not exist toasted "is not a
directory" and left the stored path at `B`. No unexpected console or page
errors.
