# Turn outputs

A turn's results — an HTML report, a chart, a table — are drawn under its
reply, so they can be read and iterated on in the conversation instead of
hunted for in the Tree. Nothing is stored for this: the session detail
(`GET /api/sessions/:id`) computes `outputs` each time it is read, which also
covers tasks that ran before the feature existed.

## Which files

`turnOutputs` (`app/internal/server/outputs.go`) reads each turn's stored
tool rows with `app/internal/toolpaths` ([022](022-transcript-file-links.md)),
which yields the files and folders the calls named, following `cd`. Inside
the project, each of those folders is listed once, not walked; dependency and
Git folders are skipped. Outside the project only a file a call named itself
counts, because other processes write to folders like `/tmp`.

A file is a turn's output when all of these hold:

- its extension is HTML, an image (PNG, JPEG, GIF, WebP, AVIF, SVG) or a
  table (CSV, TSV);
- its modification time falls between the turn's start and five seconds
  after its end (now, while it runs);
- a reply has mentioned it, in this turn or an earlier one, by path or by a
  path suffix such as its bare name.

The mention rule keeps development sessions as they were. Agents write
screenshots and scratch files to check their own work. Without the rule, 184
of the last 400 real sessions got cards, mostly such screenshots; with it, 25
of the last 600 did, each one a result the reply presented.
"I put a note on the chart" still counts, because the chart was named before.

A file rewritten later moves to the turn that rewrote it, and a deleted file
leaves. At most 12 outputs per turn and 200 listed folders per read are
kept. On the largest real sessions the whole computation takes under 30 ms.

## Cards

`TurnOutputs.vue` sits between a finished turn's last row and its usage line.
The transcript re-reads the session when a turn ends, which refreshes it.

- **HTML** is a card with the page read as text into a `srcdoc` iframe,
  sandboxed with `allow-scripts` like the HTML preview
  ([086](086-html-preview.md)), so charts, tooltips and toggles work and the
  page can be commented on ([091](091-page-annotations.md)). It starts
  480 px tall and can be resized vertically, because the frame cannot
  measure a page on another origin.
- **Tables** are a card with the CSV preview, at most 320 px tall.
- **Images** are previews up to 320 px high, served through the same preview
  route. Clicking one opens a zoomable dialog with **Circle an area** and
  **Ask in thread** ([092](092-threads.md)), anchored to the turn's last stored
  message. **Open in a tab** remains available below each image.

Each card's header names the file and its folder, turns commenting on for a
page, opens it in a kept tab on Preview, or opens it in the system browser.
HTML and table cards can be hidden; the choice lasts while the window is
open. Text and image URLs are keyed by modification time, so a rewritten
file reloads.

## Tests

Go tests cover the turn window, name-only and Markdown-link mentions, earlier
mentions, unmentioned files, dependency folders, a named file outside the
project, and a rewrite moving the output to a later turn. A browser test
drives a fake turn that writes a report, a table, an SVG and an unmentioned
screenshot, then checks the cards, script execution, hiding, circling the SVG
into a thread with an attached marked image, and opening the report in a tab.
