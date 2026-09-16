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

## Excessive desktop memory

A report of 17 GB on macOS versus 60 MB on Linux has no confirmed cause yet.
The Last Errors report does not measure memory. Compare the same workload and
process scope: the Go application, WebKit's content/GPU processes, and running
provider CLIs are separate allocations. A main-process number alone is not the
whole desktop footprint ([WebKit process architecture](https://docs.webkit.org/Deep%20Dive/Architecture/WebKit2.html)).

Current code keeps one event stream per window, with a 256-event subscriber
queue and shared, refcounted project watches. Open task tabs retain their full
transcripts; the selected transcript mounts all rows after its initial 40-row
paint. Large histories and tool payloads can therefore cost memory even with
few projects. Smart Search also retains its native model once loaded. None of
these observations establishes the cause of the reported 17 GB.

To narrow a recurrence down:

1. In Activity Monitor's Memory view, record the growing process name and PID,
   its Memory value, memory pressure and swap. Record the app version, macOS
   version, time since launch, whether tasks are running, and whether growth
   follows sleep/wake. Take a second reading after a few minutes.
2. Close long transcript/file tabs and compare growth. Let active turns finish
   to distinguish provider subprocess use from idle application use.
3. Quit the app fully (closing to tray keeps it alive). Launch the same binary
   with `--web`, open its printed URL in a browser, and repeat the workload with
   the same tabs. Compare the server and browser separately. The instance lock
   requires the desktop instance to exit first.
4. If growth is confined to desktop WebKit, collect a native memory profile;
   if the Go process grows in web mode too, collect a Go heap profile in a
   diagnostic build. Keep profiles local until reviewed: they can contain
   project and conversation data.

An [older Wails report](https://github.com/wailsapp/wails/issues/2772) describes
macOS WebKit growth after sleep on Wails 2.5.1. This app uses 2.15.0; that report
is a diagnostic lead, not evidence that the same defect is present. A fix needs
a repeated workload whose memory stops growing after the change. Native macOS
and Windows memory behavior has not been validated by this investigation.
