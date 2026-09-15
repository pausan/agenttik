# Links in transcripts and Markdown previews

Web links in agent replies follow in the browser. On the desktop, the app
opens them in the system browser because its embedded view cannot open a new
window itself. Right-clicking a web link offers **Open in browser** and **Copy
link**. In a browser deployment, a normal click keeps its usual new-tab
behaviour and the menu's open action does the same.

A file the agent names in its reply is clickable, and clicking it opens that
file in a kept app tab. An already open file is selected and kept instead of
creating a duplicate. Right-clicking a local link offers **Open in new tab**
and **Open in system browser**, plus **Copy path**. Copy uses the resolved
project-relative path, or an absolute path for a target outside the project,
without a line suffix. Directory links open the system file explorer without
creating an editor tab. System opening uses the same file opener
as the Tree ([048](048-open-in-system-browser.md)). A reference that names a
line (`web/src/store.js:801`) opens the editor scrolled to that line, with the
line selected.

Two forms are recognised, both of them what a coding agent actually writes:

- a **code span** holding a path, with or without `:line` — by far the common
  one, and the reason this is not only about markdown links;
- a **markdown link** whose target is a path, relative or absolute, with an
  optional `:801` or `#L801`.

Paths are drawn in the link colour and underline on hover. A code span keeps
its chip, so nothing about the transcript's shape changes — only some of it
became clickable.

## What is a path

Code spans require a known source, config or document extension, so
`S.detail`, `tab.temp` and `account/rateLimits/read` stay plain code.
Explicit Markdown links also accept extensionless files such as `Makefile`,
percent-encoded paths, and angle-bracket targets containing spaces.
Document fragments are stripped; `:line` and `#Lline` retain line navigation.

Transcript paths resolve from the project root. Markdown preview paths resolve
from the displayed file's folder, including `./` and `../`. Absolute paths
inside the project have the project folder trimmed off. Open and copy actions use
the same resolved path, and a missing file is reported without opening a tab.

A path that is still absolute after that trimming names a file outside the
project — another checkout, a log under `/tmp` — and agents write them often
enough that refusing to follow the link was the wrong answer. It opens, and
opens read-only: the tab offers Edit and Preview but no Diff, because no
repository of this project tracks that file, and the footer says `read only`.
The boundary that remains is the one that matters: `GET` reads the file where
it is, while saving and the Tree's create, rename and delete still take a
path relative to the project and refuse anything else.

`markdown.js` emits a `<button>`, not an `<a>`. The anchor would need an href
that means "no navigation", and the transcript is not where navigation should
be argued about; a button carries the path in a data attribute, is focusable
and answers Enter for free. `MarkdownContent.vue` shares the rendering, click
handler and context menu between message bubbles and file previews. A block
naming forty files still costs one listener per event type.

The file-info endpoint identifies directories before a local link opens,
including folders whose names have file extensions. This adds one metadata
request per local-link click.

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

- Only rendered Markdown: agent replies, thinking messages and file previews.
  Your own prompts are shown as you typed them.
- A bare path in prose is not linked, only one in a code span or a link. In
  980 lines of real transcripts there was not one bare path, and linking them
  would mean guessing at prose.
- A file that does not exist opens no tab and says why. A file outside the
  project opens read-only.
