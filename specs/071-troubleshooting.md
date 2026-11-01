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

## Linux native crashes

A `SIGSEGV` during `gtk_main` identifies the native event loop, but Go's
trace does not identify the faulting C function. Run the built desktop under
GDB to capture native thread stacks and shared-library mappings:

```sh
scripts/debug-desktop.sh >crash.log 2>&1
```

The script accepts another binary path followed by its arguments. It passes
Go's SIGURG, broken-pipe SIGPIPE and the default JavaScriptCore GC signal 34
without stopping. GDB stops at a native fault, writes registers, instructions, mappings, stacks and the library
list, then exits. The log includes process output and native stack details.
This is separate from the frontend's filtered Last Errors report.

Task keyboard investigation on WebKitGTK 2.52.6 captured a fault at library
offset `0x9b655f`, dereferencing address `0x28`, in the FormData vector
destructor. The native stack runs through `WebCore::FormData`,
`WebKit::WebURLSchemeTask`, `webkit_uri_scheme_request_finalize` and the
URI-scheme read callback in GTK's loop. This matches the reported instruction
suffix and fault address, though the original report has no library mappings.

Wails v2.16.0 releases URI request references from asset-server goroutines.
WebKit's shared request body has non-thread-safe lifetime management, so this
can race with main-loop cancellation and cleanup. Linux desktop links a
set of linker wrappers. Request-body creation and response/error completion
run synchronously on GTK's main context. URI request references and WebKit's
FormData input-stream references released outside that context are transferred
to a queued main-loop callback. The owned reference stays alive until that
callback releases it. Other GObjects and calls within system shared libraries
are unchanged. Body streams can be read on workers, but their FormData lifetime
must be managed on GTK's thread. `g_idle_add_full` always
queues; `g_main_context_invoke` can execute inline on a worker and is unsuitable
here. This works with the normal desktop `go build`; it needs no dependency
cache edits or Wails fork. Remove the wrapper when the chosen Wails v2
release guarantees main-thread request cleanup.

The same ownership issue is documented in upstream
[Wails issue 5557](https://github.com/wailsapp/wails/issues/5557).
`make test-desktop-tasks` checks the ownership rule directly and exercises
native task-dot cycling and subsequent keys on running and idle tasks.
The complete wrappers passed five consecutive native keyboard runs under GDB.
Request release alone was insufficient: the same FormData crash still occurred
until body creation and body-stream release were confined to GTK too.

## Frontend reports

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

A second macOS-only defect was fixed alongside it. fsnotify's kqueue backend
opens a descriptor per watched *file*, not per directory, and in v1.9.0
`Watcher.Close()` gave none of them back: it marked the watcher closed before
removing the paths, so every `Remove` it then called returned early. A project
of this size costs about 15k descriptors, and each watcher restart — a project
switch, or the stream re-subscribing — leaked that many again, ending in
`EMFILE` rather than in memory (about 6 MB per 42k descriptors). fsnotify
v1.10.1 drops the watches directly in `Close`; measured over five create/close
cycles, descriptors return to the baseline every time instead of climbing from
8.5k to 42k. `maxWatchDirs` bounds the walk, not the descriptor count, on
macOS.

Measuring memory here needs process scope kept straight: the Go application,
WebKit's content and GPU processes and any running provider CLI are separate
allocations, and `ps` RSS counts shared pages. `vmmap -summary`'s
`Physical footprint` is the number Activity Monitor shows
([WebKit process architecture](https://docs.webkit.org/Deep%20Dive/Architecture/WebKit2.html)).

Open task tabs still retain their full transcripts, the selected transcript
mounts all rows after its initial 40-row paint, and Smart Search retains its
native model once loaded. Large histories and tool payloads therefore still
cost memory, independently of the loop above.
