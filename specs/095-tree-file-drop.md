# Dropping desktop files on the Tree

Files and folders dragged from the desktop onto the Tree are **copied** into
the project. This works the same when the window shows another machine's
profile ([085](085-remote-profiles.md)) or is reached through the exposed
server or a quick tunnel ([043](043-exposed-server.md), [084](084-quick-tunnel.md)).

## Where it lands

Like New file ([046](046-tree-file-actions.md)): **inside** the folder under
the pointer, **beside** the file under it, and at the top of the project
when the pointer is over empty space. While dragging, the target folder's
row is outlined, or the whole pane for the top. Dropped folders keep their
structure, empty subfolders included.

If any dropped file would land on a file already in the listing, the Tree's
action dialog lists those paths and asks to **Replace** them. Cancel copies
nothing. A folder is never replaced by a file.

While copying, a line under the filter shows the file count and percentage,
with a button that stops the copy. Only one copy runs at a time. When it
ends, the listing and Changed are re-read and the first dropped entry is
selected and revealed. A failure shows a toast and stops the rest.

## How the bytes travel

The browser has no paths for dropped files, and a remote project is on
another disk, so the bytes are sent through the API. `routeRemote` forwards
these requests like any other, so one endpoint handles both cases.

`upload.js` reads the drop. It walks folders with `webkitGetAsEntry` and
falls back to `dataTransfer.files`. Each file is sent in 2 MiB pieces,
four files at a time. Pieces are `ArrayBuffer`s, not Blob slices, because
Linux WebKit can crash in Wails' URI handler on Blob bodies (the same reason
as pasted images, [056](056-pasted-images.md)). A piece stays under the 4 MiB
body limit on both this machine and the remote.

| Method | Path | Does |
|--------|------|------|
| PUT | `/api/projects/:id/upload?path=&upload=&offset=&size=[&replace=1]` | Body: one piece. 204 until the last piece, then `{path, dir}` |
| PUT | `/api/projects/:id/upload?path=&dir=1` | Makes a folder; an existing one is fine |
| DELETE | `/api/projects/:id/upload?path=&upload=` | Removes a stopped copy's partial file |

`upload` is a 32-hex token chosen by the client (from `getRandomValues`, since
`randomUUID` needs a secure context). Pieces are appended to
`.agenttik-<token>.part` beside the target. The last piece renames it into
place. A failed or stopped copy never leaves half a file under the real name
and never destroys the file it was replacing. A piece must start at the
partial file's current length (409 otherwise). Offset 0 truncates, so a
first piece can be sent again. Every piece refuses an existing target unless
`replace=1`, and always refuses an existing folder. Paths go through
`resolveEntry`, so they cannot leave the project or touch `.git`.

## Validation

Go tests cover pieces in order, an out-of-order piece, empty files, folders,
replace rules, bad input, cancel, and a copy forwarded to a second real
instance. `upload.test.js` covers folder walking, the fallback, conflict
names, piece requests and cleanup after a failure. `tree-file-drop.spec.js`
drops onto a folder, beside a top-level file, and over an existing file
(cancel, then replace), and drops onto a remote machine's Tree.
