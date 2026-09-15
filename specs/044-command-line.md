# The command line, and the version a tag builds in

`agenttik` takes long options: `--addr`, `--data-dir`, `--init`, `--web`,
`--private`, `--remote`, `--api`, `--body`, `--help` and `--version`. Go's `flag` package treats `-addr` and `--addr` as
the same option, so the single-dash form keeps working; only the double-dash
one is documented. `--help` prints to stdout and exits 0, while an unknown
option prints the same text to stderr and exits 2, which is `flag`'s own
behaviour.

`main.usage` writes that text rather than the `flag` default, so the options
appear with two dashes. It walks `flag.VisitAll`, so an option added later
lists itself, with the placeholder taken from the backquoted word in its usage
string (`--addr host:port`) and the default appended unless it is empty or
`false`.

Private instances use temporary storage and an independent lock; see
[066](066-private-mode.md). Remote client launches are described in
[064](064-remote-connections.md).

## `--init`: adding a project from the terminal

`agenttik --init` adds a folder to the project list and exits. With no operand
it adds the folder the terminal is in, which is the whole point — you are
already standing in the project you want, so neither the path nor the Add
Project dialog is worth typing. `agenttik --init path/to/repo` adds that one
instead.

It works whether or not the app is open, because whether it happens to be open
is beside the question being asked. Which of the two paths runs is decided by
the same lock as a second launch ([039](039-single-instance.md)):

| State | What `--init` does |
|-------|--------------------|
| Nothing running | takes the lock, writes the row itself, releases, exits |
| An instance running | `POST /api/projects` to the address in the lock file |
| Lock held, no address published yet | `agenttik is starting up; try again in a moment`, exit 1 |

The instance that owns the database is the one that must write to it, which is
what the second row is for. It also means an open window redraws its sidebar
the moment the command returns: `POST /api/projects` publishes
`projects_changed`, which every stream carries ([004](004-ui.md)).

`--init` adds a project; it does not start the app. A launch with nothing
running would otherwise be two commands in one, and the one you asked for is
over in a millisecond.

The folder is resolved to an absolute path *before* it is sent, since the
instance receiving it was started somewhere else entirely and a relative path
means nothing by the time it arrives. A folder that does not exist, or is a
file, fails locally and is never sent. A folder that is already a project
prints `<path> is already a project` and exits 0: running `--init` twice is
not an error, the folder is a project either way.

`flag` has no option that takes an optional value, so `--init` is a boolean
and the folder is the operand after it — `flag` stops parsing at the first
operand, leaving it in `flag.Arg(0)`. The help text still writes it
`--init folder`, since that is the form to type. `--init=folder` is the one
spelling that does not work, and `flag` rejects it as a bad boolean.

## `--api`: controlling the running app

`agenttik --data-dir DIRECTORY --api METHOD [--body JSON|-] /api/path` sends a
request to the app using that data directory. All flags precede the path.
Supported methods are GET, POST, PUT, PATCH and DELETE; the path may include
query parameters and must start with `/api/`. `--body -` reads JSON from stdin,
including embedded newlines. Bodies are validated and capped at 4 MiB.

The client reads the address published in the lock file; it does not acquire
the lock, open the database or launch an app. Discovery therefore needs only
read access to the data directory. An absent address or a connection failure
reports an error. The client uses loopback when the server published an
all-interface listener, refuses redirects and has a 30-second timeout.

Successful response bodies go unchanged to stdout; HTTP errors go to stderr
with a nonzero exit status. A command that starts a turn returns when accepted,
without waiting for the provider to finish. The orchestrator's built-in prompt
documents the requests it uses ([054](054-orchestrator-project.md)).

## Where the version comes from

`main.version` defaults to `dev` and the build overwrites it with
`-ldflags "-X main.version=..."`. `--version` prints `agenttik <version>`.

A release tag is `vMAJOR.MINOR.PATCH`, optionally followed by letters, dashes
and further dots: `v1.2.3`, `v1.2.3-beta.1`, `v1.2.3-rc-2`. Only the
`MAJOR.MINOR.PATCH` part is built in, so `--version` always reports a bare
`X.Y.Z`. The suffix survives in the asset names and the release title, which
use the whole tag; two tags sharing a core therefore ship binaries that report
the same version.

An untagged push has no version to report, so it reports which commit it is:
the first 12 characters of the SHA. `make` derives the same two cases from
`git describe --tags --exact-match` on HEAD, falling back to the short commit,
and `make build VERSION=1.2.3` overrides both.
