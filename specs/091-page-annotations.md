# Comments on previewed pages

A page an agent built — a report, a chart, a mockup — can be commented on
where it is shown: in an output card under a reply
([090](090-turn-outputs.md)) or in an HTML tab on Preview
([086](086-html-preview.md)). **Comment** turns commenting on; the pointer
outlines the element under it, and a click opens a small form under that
element instead of reaching the page. **Add to prompt** (or Enter) appends the
comment to the draft of the task the view belongs to, and commenting stays on
for the next one. Escape closes the form, and a second Escape, or the button,
turns commenting off. Several comments can be collected and sent as one
prompt, with any attached images kept.

What reaches the prompt names the file and the element in words the agent
can find in the source:

```
On `tasks/r/report.html`, at the <button> "Month (since Jan 2025)" under "GH+ app — install → trial" (`body > div.viz-root > div.controls:nth-of-type(1) > button:nth-of-type(2)`, data-g="month"):
Default to Month here.
```

The element's label is its `aria-label`, `title`, `alt` or SVG `<title>`,
else its text, cut to 80 characters. The heading is the last `h1`–`h6`
before it. The selector climbs at most six levels and stops at an id. Up to
six `data-*` attributes follow, since charts often keep their values there.

## How

The page runs on an opaque origin, so the app cannot read what was clicked.
`htmlPreviewDocument` therefore puts a small script, `ANNOTATOR`
(`web/src/html-preview.js`), at the top of every previewed document. It does
nothing until the parent posts `{agenttik: "annotate-mode", on}`. While on,
it outlines the element under the pointer, swallows pointer, mouse and click
events in the capture phase, and posts `{agenttik: "annotate", element}` with
the description and the element's rectangle. The mode is posted again when
the frame loads, so a redraw keeps it.

`HtmlFrame.vue` holds the iframe and the form. It accepts messages only from
its own frame and keeps only bounded strings and numbers, because the page's
own scripts can post too: at worst a page can describe an element wrongly,
which the form shows before anything is added. Output cards load their page
as text into `srcdoc`, like the HTML tab, so both carry the annotator.
`addToPrompt` (`web/src/store.js`) appends to the owning task's draft; the
button is offered only where a task owns the view. On a file tab the prompt
bar is out of sight, so a notice says where the comment went.

## Limits

- Pages reached by navigating inside the frame are served by the preview
  route without the annotator, so they cannot be commented on.
- The comment describes the element; it carries no screenshot.
- A page that stops event propagation in its own capture listeners on
  `window` before the annotator's can still receive the click.

## Tests

The output-card browser test turns commenting on, clicks the report's
heading, checks the page's own click handler did not run, adds a comment and
reads the prompt, then turns commenting off and checks the page's handler
runs again. It also checks the HTML tab offers **Comment**.
