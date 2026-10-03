# Previewing SVG and images

A file tab can now render pictures, not only text.

**An SVG previews and stays editable.** `.svg` joined `PREVIEWABLE`, so the tab
carries `Edit | Diff | Preview` exactly as markdown does: the text is the file
and the picture is a second view of it. `FilePreview` draws it through an
`<img>` whose `src` is a `data:image/svg+xml` URL built from the tab's own
text, so the drawing follows the editor keystroke by keystroke and an SVG's
Diff is still the text diff, unified or split.

**A raster image previews and has no editor.** `isImage()` covers `png`, `jpg`,
`jpeg`, `gif`, `webp`, `avif`, `bmp`, `ico` and `apng`. Such a tab offers
`Diff | Preview` and nothing else — `setFileMode` refuses `edit`, `openingMode`
opens on the picture when the remembered view is one an image cannot show, the
tab is `readOnly` from the moment it is built so no Save button appears, and
`gotoLine` leaves it alone since a line means nothing in a picture. Its text is
never fetched: `loadFileTab` short-circuits, and the bytes go straight from the
raw endpoint into the element that draws them. A separate small metadata
response supplies the header's [file size and color depth](059-file-information.md).

**An image's Diff is the two pictures, side by side, and only that.** `ImageDiff`
replaces `FileDiff` for an image and carries no unified/split toggle: there is
no unified shape of a picture. The left is the baseline — `HEAD` for the working
tree, the commit's parent for a file opened from a commit — and the right is
what it is now. Each side prints its own pixel size underneath, which is most
of what a picture's diff has to say, and is why a raster image is drawn at its
natural size rather than stretched to its pane: two images both filled to their
half would look the same size when they are not. A vector has no pixels to
blur, so an SVG preview does fill the pane (`fit`).

**A preview zooms, and so does a diff of two pictures the same size.**
`ImageFrame` takes `zoomable`. The picture opens fitted to its pane. The `−`,
level and `+` buttons under it step along `ZOOMS` in `image-zoom.js` (5% to
3200%); the level reads `Fit` until the first zoom and a click on it fits
again. `Ctrl` with the wheel zooms smoothly and so does a trackpad pinch, which
browsers send as that same event; the wheel alone still scrolls. On a touch
screen two fingers pinch: the pane sets `touch-action: pan-x pan-y`, so the
page itself does not zoom and one finger still scrolls. Every zoom keeps the
point under the pointer, the fingers' midpoint, or the pane's middle where it
was, and a picture larger than the pane scrolls in both directions. An SVG
with only a `viewBox` has no size of its own, so its 100% is its fitted size.
A new source opens fitted.

**The zoom chords are the desktop window's only.** There, `Ctrl`/`Cmd` with
`+` or `=` and `-` or `_` step the zoom and `0` sets 100%, the image's own
pixels. They are read on the window, so they act without a click on the
picture first, but not while a text field, editor or terminal has focus, and
not while the frame is hidden. In a browser (no `window.runtime`) they are left
alone and stay the browser's own page zoom.

**Two pictures the same size zoom and scroll as one.** `ImageDiff` learns each
side's size as it loads. When they match, both frames get one shared `view`
(`scale`, `left`, `top`): a zoom on either side zooms both, and a scroll on one
writes its position, which the other follows; a position already held ends the
exchange. Only the right side, the one that stands for the file now, answers
the chords. Two pictures of different sizes have no common spot to keep
aligned, so they stay fitted with no zoom controls. An added or deleted image
shows one side, and that side zooms as a preview does.

`GET /api/projects/:id/raw?path=&rev=` serves the bytes. `rev` is optional and
reads the file from a revision instead of the working tree — that is what the
baseline side of a diff asks for. A side that does not exist is a plain 404 and
draws as a note rather than a broken picture, and the two cases that are normal
rather than exceptional are read off the diff text instead: git heads a new
file with `new file mode` and a removed one with `deleted file mode`, so
`ImageDiff` says "Added — nothing before it." or "Deleted." without spending a
request that could only fail.

## Choices

**The endpoint serves images and [fonts](058-font-preview.md).** `rawTypes` is an
allowlist of extension to content type, so nothing is sniffed and no other file
in a project can be fetched as same-origin bytes. **SVG is deliberately not in
it**: it is markup that can carry scripts, and a URL serving it same-origin
could be opened on its own. The SVG preview renders the open tab's text
instead, which is both safer and better — it follows the edits.

**An `<img>` rather than a sandboxed iframe for SVG.** The HTML preview needs an
iframe because it is a document. An SVG in an `<img>` already cannot run a
script or fetch anything, which is the boundary the iframe was drawn for, at no
cost and with the size available to read off the element.

**The image diff still fetches the text diff.** Only for its one useful word:
git prints nothing at all when a file has not changed, which is what keeps
"No changes to this file." working, and `new file mode` / `deleted file mode`
say which sides exist. For a real image this is under 100 bytes — git detects
binary and prints one line — so it costs a request that was already being made.

**`rev` is matched against a pattern, not escaped.** `^(HEAD|[0-9a-f]{4,40})\^?$`
is the only client value handed to git as a revision, the same rule
`commitTarget` already applies to a commit hash, with `^` added because the
baseline side of a commit's diff is its parent.

**`git show` output is read as bytes, through a cap.** `runGit` returns a
string, and an image is not one. `capped` refuses output past the 16 MiB
preview limit while it streams rather than after it is all in memory.
