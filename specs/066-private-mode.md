# Private mode

`agenttik --private` starts a separate instance in a fresh, owner-only temporary
data directory. It does not open the normal database or instance lock. Without
an explicit `--addr`, web mode chooses an available loopback port.

The directory, including SQLite files and app-managed data, is removed after
shutdown or a startup error. SIGINT and SIGTERM shut down web mode normally.
Forced termination (SIGKILL), crashes, and power loss cannot run cleanup.
Project working files and external provider state are not erased.

`--private` cannot be combined with `--init`, `--api`, or `--remote`.
`--data-dir` is ignored in private mode.

Validation: configuration tests cover unique directories, removal, and preservation
of normal data.
