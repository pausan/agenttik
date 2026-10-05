# Binary files and Hex

Working files containing NUL bytes or invalid UTF-8 open in Hex instead of
the text editor. Images, fonts and videos offer Hex beside Diff and Preview;
their usual tree action still opens Preview. SVG stays editable text.
Commit tabs retain their existing Diff view. Git's binary summary and image
diffs remain available; there is no byte-level binary diff.

Hex shows offsets, 16 bytes per row and printable ASCII (other bytes use a
dot). Only 512 bytes / 32 rows render at a time. Previous/Next page and a
hexadecimal offset field navigate the loaded bytes. Clicking a byte selects
its address. Replace bytes accepts complete hex pairs, with optional spaces,
and overwrites bytes starting at that address without changing the file size.
Local Undo/Redo store byte patches, not copies of the whole file. Ctrl/Cmd+Z
and Y work while the grid or its buttons have focus; text inputs keep their
own native undo. Returning to a tab keeps its selected offset and edits.

Edits pin temporary tabs and use the existing Save / Ctrl+S, unsaved-close
dialog and quit protection. Replacing bytes with their original values clears
the dirty marker. A successful save refreshes the saved baseline and file
metadata. Hex does not become the default mode for subsequent text files.

The existing file endpoint reads at most 2 MiB. Larger binaries show the first
2 MiB, marked read-only; offsets beyond that prefix cannot be visited.
Files outside the project also stay read-only. Media preview limits and video
streaming remain separate from this hex limit.

`GET /api/projects/:id/file?path=` adds `hex` and `version` for binary files.
`format=hex` forces a byte view for media, including empty files. `hex` is a
lowercase encoding of the loaded bytes; `version` is their SHA-256 hash.
Text detection excludes an incomplete UTF-8 rune split by the preview cap.

`PUT /api/projects/:id/file?path=` accepts exactly one of `content` or `hex`.
Hex writes require the loaded `version`, an existing file, valid pairs, the
same byte length and no more than 2 MiB. A changed hash returns 409 and leaves
edits in the tab. Saves use atomic replacement and preserve file permissions.
The hash is checked before writing, not an OS-level compare-and-swap against
concurrent external writers. Project path and symlink checks still apply.
The request body cap includes space for a full hex encoding plus JSON fields.

Go tests cover bytes, permissions, stale versions, invalid input, read-only
limits, outside paths, empty files, forced hex, text detection and an exact
2 MiB save. Web unit tests cover parsing, bounded rows and replacement.
Browser tests cover navigation, ASCII/hex rendering, edits, undo/redo, tab
switches, repeated saves and media switching.
