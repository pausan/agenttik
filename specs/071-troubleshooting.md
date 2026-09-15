# Troubleshooting

Help → Troubleshooting → Last Errors lists up to 100 errors, newest first,
from the last five app launches that recorded errors. A launch is a page load,
not a conversation. Copy report and Copy all errors produce JSON bug reports;
Clear errors removes the history. Nothing is sent automatically.

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
reload persistence, copying and clearing in Help.
