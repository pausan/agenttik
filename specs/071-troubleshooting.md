# Troubleshooting

Settings → About → Troubleshooting → Last Errors lists up to 100 errors, newest
first, from the last five app launches that recorded errors. A launch is a page
load, not a conversation. Copy report and Copy all errors produce JSON bug
reports; Clear errors removes the history. Nothing is sent automatically.

The frontend records failed API requests (including network and JSON errors),
the shared Something went wrong handler, Vue errors, uncaught browser errors,
unhandled promise rejections and profile startup failures. The same Error
object is recorded once. Provider transcript errors and native process crashes
are not captured by this frontend log.

Reports use an allowlist: timestamp, random launch ID, error class, capture
source, HTTP method/status, fixed API resource category, and up to twelve stack
line/column locations. Raw messages, stack function names and file URLs, request
bodies, prompts, query strings, project/task/account identifiers and arbitrary
error properties are never saved. Unknown categories become generic values.
This limits diagnostic detail in exchange for excluding free-text private data.

History uses existing instance/profile-scoped browser storage. Private mode
keeps it in memory. Before profile mode is known, startup errors stay in memory.
Malformed history is ignored and stored fields are validated again when read
or copied. Storage failures never mask the original error.

Validation: frontend unit tests cover privacy, duplicate capture, retention,
corrupt storage and unavailable storage. The browser test covers capture,
reload persistence, copying and clearing in About.

## Idle CPU and desktop memory on macOS

An idle macOS window used to hold about a third of a core and climb in memory
for as long as it was open, while the same build on Linux sat still. The cause
was a refresh loop between the file watcher and git, described in
[019](019-file-watching.md): reading `.git/index` bumps its access time, kqueue
reports `NOTE_ATTRIB`, fsnotify delivers `Chmod`, and the refresh that answered
it ran `git status`, which read the index again. `--no-optional-locks` stops
git writing the index but not the access-time bump, so the flag alone did not
break the cycle. inotify does not raise the matching event for a plain read,
which is why only macOS spun.

Measured on a 15k-file project with one window open and nobody touching it:

| | before | after |
|-----|-----|-----|
| `files_changed` in 60 s | 148 | 0 |
| main process CPU | ~33% of a core | ~0.05% |
| WebKit content process CPU | ~5.5% | ~0.03% |
| main process footprint | 119–133 MB, sawtoothing | ~101 MB, flat |

Each turn of the loop walked the whole tree, serialized it to JSON, compressed
it and pushed it through the macOS `WKURLSchemeTask` bridge, so the cost grew
with the size of the project rather than with anything the user did.

A separate macOS-only defect remains open: fsnotify's kqueue backend opens a
descriptor per watched *file*, not per directory, and `Watcher.Close()` does
not release them. One project of this size costs about 15k descriptors, and
each watcher restart — a project switch, or the stream re-subscribing — leaks
that many again. It is bounded by the descriptor limit rather than by memory
(about 6 MB per 42k descriptors), so it is a resource leak, not the growth
above. `maxWatchDirs` caps directories, which bounds nothing on macOS.

Measuring memory here needs process scope kept straight: the Go application,
WebKit's content and GPU processes and any running provider CLI are separate
allocations, and `ps` RSS counts shared pages. `vmmap -summary`'s
`Physical footprint` is the number Activity Monitor shows
([WebKit process architecture](https://docs.webkit.org/Deep%20Dive/Architecture/WebKit2.html)).

Open task tabs still retain their full transcripts, the selected transcript
mounts all rows after its initial 40-row paint, and Smart Search retains its
native model once loaded. Large histories and tool payloads therefore still
cost memory, independently of the loop above.
