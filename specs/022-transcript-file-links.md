# Links in a transcript

Web links in agent replies follow in the browser. On the desktop, the app
opens them in the system browser because its embedded view cannot open a new
window itself. Right-clicking a web link offers **Open in browser** and **Copy
link**. In a browser deployment, a normal click keeps its usual new-tab
behaviour and the menu's open action does the same.

A file the agent names in its reply is clickable, and clicking it opens that
file in a tab — the project's temporary tab, the same one a click in the Tree
uses, so reading through what an agent changed leaves one tab behind rather
than twenty. A reference that names a line (`web/src/store.js:801`) opens the
editor scrolled to that line, with the line selected.

Two forms are recognised, both of them what a coding agent actually writes:

- a **code span** holding a path, with or without `:line` — by far the common
  one, and the reason this is not only about markdown links;
- a **markdown link** whose target is a path, relative or absolute, with an
  optional `:801` or `#L801`.

Paths are drawn in the link colour and underline on hover. A code span keeps
its chip, so nothing about the transcript's shape changes — only some of it
became clickable.

## What is a path

An extension is what separates a path from everything else a code span holds.
Without that rule `S.detail`, `tab.temp` and `account/rateLimits/read` all
read as files; with it, none of them do and `README.md` still does. The list
is source, config and document types — the files an agent talks about.

The path is passed to the project's file API as it stands, so a path outside
the project is refused there rather than guessed at here. An absolute path
inside the project is the exception: the project's own folder is trimmed off
the front, because that is the same file said a longer way.

`markdown.js` emits a `<button>`, not an `<a>`. The anchor would need an href
that means "no navigation", and the transcript is not where navigation should
be argued about; a button carries the path in a data attribute, is focusable
and answers Enter for free. `Message.vue` listens once per bubble, so a reply
naming forty files still costs one listener.

## Finding the line

Lines wrap, so the 30,000th character is not the 800th line down and no
arithmetic on the text can say where a line sits. It has to be measured, and
the coloured layer under the editor is what can be measured: a DOM range from
the top of the text to the line's first character ends exactly where that
line begins. The pane is then scrolled to put it a third of the way down.

Selecting the line in the textarea is what marks it. Focus is taken with
`preventScroll`, because focusing a field as tall as the whole file would
otherwise reveal its top.

`goto` on the tab is a request rather than state: the editor serves it and
clears it, so the same reference clicked twice scrolls to it twice. A tab
left on Diff or Preview is switched to the editor — a line means nothing in
the other two — without moving the remembered view, which is the user's
choice and not one a link should make for them.

## Limits

- Only what is rendered as markdown, which is the agent's replies. Your own
  prompts are shown as you typed them.
- A bare path in prose is not linked, only one in a code span or a link. In
  980 lines of real transcripts there was not one bare path, and linking them
  would mean guessing at prose.
- A file that does not exist, or a path outside the project, opens no tab and
  says why.
