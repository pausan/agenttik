# Pasted prompt images

Ctrl+V (or the platform paste command) in the prompt accepts one or several
clipboard images. Repeated pastes add images. Each has a preview and remove
button. Text-only paste remains native; mixed clipboard text is inserted at
the selection. Send, Enqueue and Schedule wait for uploads to finish.
The handler reads images from the paste event's items or files. Linux WebKit
can leave both lists empty for an image paste; in that case it reads images
through `navigator.clipboard.read()` during the paste gesture. It reads one
image representation per clipboard item and does not read the clipboard for
ordinary text paste.

PNG, JPEG, GIF and WebP are accepted, up to 4 MiB each. The browser uploads
each file as bytes to `POST /api/attachments`.
The upload materializes an `ArrayBuffer` after checking the size limit; it
never passes a `Blob` or `File` as the fetch body, because Linux WebKit's
native URI-scheme request body reader can crash on those bodies.
The server checks the content signature and saves it as
`attachments/<sha256>.<extension>` in the app data
directory, outside project trees. Identical images share a file.
`GET /api/attachments/:name` serves previews through the normal server access
controls. Arbitrary filenames are refused.

Drafts contain `![Attached image](/api/attachments/<name>)` references, hidden
from the prompt text box and shown as previews. This uses the existing draft,
queue, schedule, transcript and retry storage; images alone are a valid prompt.
The transcript renders references as links. Before starting a turn, the runner
resolves references to absolute local paths and asks the agent to use its
image-reading tool. Image understanding requires a provider with that tool
and image support; images are not sent as native multimodal API content.

Images are retained across restarts for queued work and saved conversations.
Removing a draft attachment removes its reference, not the shared disk file.
There is currently no automatic attachment cleanup.

`make test-desktop-images` exercises Blob and File uploads through the real
Linux Wails/WebKit request handler in an isolated process under Xvfb.
The UI unit tests also check byte preservation and rejection before reading
oversized images. Browser paste coverage lives in `e2e/tests/pasted-images.spec.js`.
