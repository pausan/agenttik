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

## The Linux desktop window

WebKitGTK shows the page a desktop file drag only as `text/uri-list`, with
no `Files` type, no File objects and an empty URI. A page that does not take
such a drag lets WebKit open the file in place of the app. So:

- Any drag carrying `Files` or `text/uri-list` counts as a desktop drag
  (`carriesFiles`). The Tree takes it, and a window-level listener in
  `main.js` refuses it everywhere else, so no drop navigates the window.
- On Linux, `configureDesktop` sets Wails' `EnableFileDrop`. The shell passes
  the native drop's paths to `Server.DropFiles` and emits
  `agenttik:file-drop` with the pointer position. The Tree aims at the
  element there, as it does on `dragover`, and `readDesktopDrop` reads the
  files back to feed the same upload.
- `GET /api/desktop/drop` lists the dropped files (`{files: [{path, size}],
  dirs}`). `GET /api/desktop/drop/file?path=&offset=&length=` returns one
  piece. Both stay on this machine (`/api/desktop`). They answer only
  requests with the `X-Agenttik-Window-Token` the window's proxy adds, and
  only paths inside what was last dropped.

macOS and Windows webviews give the page real File objects, so they use the
browser path. `--remote` desktop windows have no local server to read the
paths, so a native drop there is refused like any other.

## Validation

Go tests cover pieces in order, an out-of-order piece, empty files, folders,
replace rules, bad input, cancel, and a copy forwarded to a second real
instance. They also cover the drop endpoints' token, listing, pieces and
path checks. `upload.test.js` covers desktop-drag detection, reading a
native drop, folder walking, the fallback, conflict names, piece requests
and cleanup after a failure. `tree-file-drop.spec.js`
drops onto a folder, beside a top-level file, and over an existing file
(cancel, then replace), and drops onto a remote machine's Tree. The Linux
native path was checked by hand: the desktop app under Xvfb, with a GTK drag
source driven by xdotool. A file and a folder were copied, and a drop outside
the Tree left the app in place.
