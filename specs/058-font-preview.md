# Font previews

`.ttf`, `.otf`, `.woff` and `.woff2` files, matched without case sensitivity,
offer Diff and Preview. They open in Preview unless the remembered file view
is Diff. They are read-only, with no Edit, Save or line count, and line links
cannot switch them into the editor. Committed files retain their Diff-only
view; font diffs show git's binary change description.

Preview loads the file through the raw endpoint into a browser `FontFace`.
The server serves `font/ttf`, `font/otf`, `font/woff` and `font/woff2`, with the
existing repository path checks, 16 MiB limit, `no-store` and `nosniff` headers.
The text-file endpoint is never fetched for a font preview. A loading note
appears until the face is ready; a failed load shows an error instead of a
sample in a fallback face. Each mounted preview has its own font family and
removes its face when its source changes or the preview unmounts. Late loads
from a previous source cannot register a stale face.

The default sentence is “The quick brown fox jumps over the lazy dog.
0123456789”. Sample text is editable and Reset text restores that sentence
without changing the selected size. Size is in CSS pixels, defaults to 48,
and is bounded to 8–160. The sentence stays on one line and scrolls
horizontally when needed. Controls wrap in narrow panes. Sample and size
belong to the mounted preview and reset when another font is opened.

The glyph grid shows the 94 printable non-space ASCII characters at the same
size. Each has a faint 1 em square and ¼ em padding on a light background.
These are em/line-box guides, not measured ink bounds or font side bearings.
The grid is a sample, not a full character map; missing characters use the
browser's usual fallback. Users can enter other scripts in the sample line.

Go tests cover font bytes, MIME types and rejection of active document types.
The browser test loads real TTF, CFF OTF, WOFF and WOFF2 fixtures, checks font
metrics, custom/reset text, size bounds, square dimensions, narrow controls,
cleanup and failed-load recovery.
