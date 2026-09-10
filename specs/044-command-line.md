# The command line, and the version a tag builds in

`agenttik` takes long options: `--addr`, `--data-dir`, `--web`, `--help` and
`--version`. Go's `flag` package treats `-addr` and `--addr` as the same
option, so the single-dash form keeps working; only the double-dash one is
documented. `--help` prints to stdout and exits 0, while an unknown option
prints the same text to stderr and exits 2, which is `flag`'s own behaviour.

`main.usage` writes that text rather than the `flag` default, so the options
appear with two dashes. It walks `flag.VisitAll`, so an option added later
lists itself, with the placeholder taken from the backquoted word in its usage
string (`--addr host:port`) and the default appended unless it is empty or
`false`.

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
