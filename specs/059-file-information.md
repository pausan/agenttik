# File information

The central file panel shows a parenthesized size beside the path in Edit,
Diff and Preview: `(32.4 KB)`. Units are decimal (1000 bytes per KB), from B
through EB, with exactly one decimal place, including `(0.0 B)` for an empty
file. The size describes saved bytes, including for binary and truncated files;
typing leaves it unchanged and saving refreshes it.

Raster images add dimensions and stored bits per pixel where known:
`(32.2 KB, 320x420 24-bit)`. PNG/APNG, JPEG, GIF, BMP, ICO and WebP headers
provide this metadata without decoding pixels. Alpha counts toward depth;
palette images show index depth. ICO reports its largest directory entry.
SVG and AVIF previews supply dimensions from the browser and omit bit depth.
Unknown or unreadable headers keep the file size alone.

`GET /api/projects/:id/file-info?path=&rev=` returns `size` and an optional
`image: {width, height, bits}`. Working files need only a stat and, for images,
at most 64 KiB of header bytes. Revisions use Git's blob size; image headers
are read from blobs within the existing 16 MiB preview limit. The route uses
the same project boundary and nearest-repository resolution as file diffs.

FileView fetches metadata when a file, its saved content, or its diff changes.
Late responses cannot overwrite a reused tab. Metadata failure does not block
the editor or preview. Commit tabs read their revision; deleted diffs read the
parent/HEAD file. The header can wrap in a narrow panel.

Header layouts follow the [PNG specification](https://www.w3.org/TR/png-3/),
[WebP container specification](https://developers.google.com/speed/webp/docs/riff_container)
and [bitmap header definition](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/ns-wingdi-bitmapinfoheader).

Unit tests cover formatting, image depths, short headers, exact file sizes,
path validation and revision metadata. The browser test covers text, images,
Diff, reused tabs, empty files, SVG dimensions, narrow headers and size refresh
after saving UTF-8 text.
