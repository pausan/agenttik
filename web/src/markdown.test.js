import { test } from "node:test";
import assert from "node:assert/strict";

import { COPY_BUTTON, drawBlocks, fileRefs, markdown, markdownBlocks } from "./markdown.js";

const code = (pre) => `<div class="code">${pre}${COPY_BUTTON}</div>`;

test("pasted image references open the stored image instead of the project editor", () => {
  const url = `/api/attachments/${"a".repeat(64)}.png`;
  assert.match(markdown(`![Attached image](${url})`), new RegExp(`<a href="${url}"`));
  assert.doesNotMatch(markdown("![Attached image](/api/attachments/../../secret.png)"), /<a /);
});

test("inline emphasis, code and strikethrough", () => {
  assert.equal(markdown("**bold** and *thin* and `x=1` and ~~gone~~"),
    "<p><strong>bold</strong> and <em>thin</em> and <code>x=1</code> and <del>gone</del></p>");
  assert.equal(markdown("__bold__ and _thin_"),
    "<p><strong>bold</strong> and <em>thin</em></p>");
});

test("markers inside a code span are literal", () => {
  assert.equal(markdown("`**not bold**`"), "<p><code>**not bold**</code></p>");
});

test("an underscore inside a word is not emphasis", () => {
  assert.equal(markdown("call some_long_name now"), "<p>call some_long_name now</p>");
});

test("html in the source is escaped, never emitted", () => {
  assert.equal(markdown('<img src=x onerror="alert(1)">'),
    "<p>&lt;img src=x onerror=&quot;alert(1)&quot;&gt;</p>");
  assert.equal(markdown("```\n<script>bad()</script>\n```"),
    code("<pre><code>&lt;script&gt;bad()&lt;/script&gt;</code></pre>"));
});

test("a fenced block keeps its text and names its language", () => {
  assert.equal(markdown("```go\nfunc main() {}\n```"),
    code('<pre><code class="language-go">func main() {}</code></pre>'));
});

test("an unclosed fence still renders", () => {
  assert.equal(markdown("```\nstill here"), code("<pre><code>still here</code></pre>"));
});

test("headings and rules", () => {
  assert.equal(markdown("# One\n\n## Two\n\n---"),
    "<h1>One</h1><h2>Two</h2><hr>");
});

test("a bullet list right under a paragraph starts a list", () => {
  assert.equal(markdown("Steps:\n- one\n- two"),
    "<p>Steps:</p><ul><li>one</li><li>two</li></ul>");
});

test("ordered lists stay ordered and separate from bullets", () => {
  assert.equal(markdown("1. one\n2. two\n\n- three"),
    "<ol><li>one</li><li>two</li></ol><ul><li>three</li></ul>");
});

test("an indented bullet nests inside the item above", () => {
  assert.equal(markdown("- one\n  - deep\n- two"),
    "<ul><li><p>one</p><ul><li>deep</li></ul></li><li>two</li></ul>");
});

test("blockquotes are parsed as blocks of their own", () => {
  assert.equal(markdown("> quoted **text**"),
    "<blockquote><p>quoted <strong>text</strong></p></blockquote>");
});

test("pipe tables become tables", () => {
  assert.equal(markdown("| a | b |\n|---|---|\n| 1 | 2 |"),
    "<table><thead><tr><th>a</th><th>b</th></tr></thead>" +
    "<tbody><tr><td>1</td><td>2</td></tr></tbody></table>");
});

test("links open outside the app and drop unsafe schemes", () => {
  assert.equal(markdown("[docs](https://example.com)"),
    '<p><a href="https://example.com" target="_blank" rel="noreferrer noopener">docs</a></p>');
  assert.equal(markdown("[x](javascript:alert)"), "<p>x</p>");
  assert.equal(markdown("[y](data:text/html,hi)"), "<p>y</p>");
  assert.equal(markdown("![shot](https://example.com/a.png)"),
    '<p><a href="https://example.com/a.png" target="_blank" rel="noreferrer noopener">shot</a></p>');
});

test("a path becomes a button that opens the file, a name that is not does not", () => {
  assert.equal(markdown("in `web/src/store.js:801` now"),
    '<p>in <button type="button" class="file" data-file="web/src/store.js" data-line="801" ' +
    'title="Open web/src/store.js:801"><code>web/src/store.js:801</code></button> now</p>');
  assert.equal(markdown("[the handler](app/internal/server/api_files.go)"),
    '<p><button type="button" class="file" data-file="app/internal/server/api_files.go" ' +
    'title="Open app/internal/server/api_files.go">the handler</button></p>');
  // A code span holds far more than paths, and only paths are clickable.
  assert.equal(markdown("`S.detail` and `account/rateLimits/read` and `npm run build`"),
    "<p><code>S.detail</code> and <code>account/rateLimits/read</code> and " +
    "<code>npm run build</code></p>");
});

test("a bare url is linked without its trailing punctuation", () => {
  assert.equal(markdown("see https://example.com/a."),
    '<p>see <a href="https://example.com/a" target="_blank" rel="noreferrer noopener">' +
    "https://example.com/a</a>.</p>");
});

test("a single newline inside a paragraph is kept as a break", () => {
  assert.equal(markdown("one\ntwo"), "<p>one<br>two</p>");
});

test("empty input renders nothing", () => {
  assert.equal(markdown(""), "");
  assert.equal(markdown(undefined), "");
});

test("explicit local links accept extensionless files, spaces and document fragments", () => {
  for (const [target, path] of [
    ["myfile.md", "myfile.md"],
    ["Makefile", "Makefile"],
    ["docs/my%20file.md", "docs/my file.md"],
    ["<docs/my file.md>", "docs/my file.md"],
    ["../README.md#overview", "../README.md"],
  ]) {
    assert.match(markdown(`[blah](${target})`), new RegExp(`data-file="${path.replaceAll(".", "\\.")}"`));
  }
  assert.match(markdown("[line](<docs/my file.md:12>)"), /data-line="12"/);
  for (const target of ["javascript:alert", "data:text/plain,hello", "//example.com/a.md", "bad%00.md", "bad%ZZ.md"]) {
    assert.equal(markdown(`[blah](${target})`), "<p>blah</p>");
  }
});

test("sound files in code spans link, and fileRefs lists each linked file once", () => {
  const html = markdown("Made `out/song.mp3` and [this](a&b.WAV), see `out/song.mp3` and `notes.md`.");
  assert.match(html, /data-file="out\/song\.mp3"/);
  assert.deepEqual(fileRefs(html), ["out/song.mp3", "a&b.WAV", "notes.md"]);
});

test("markdownBlocks gives one element per block, which markdown joins", () => {
  const src = "# Title\n\nSome **text**.\n\n- a\n- b\n\n```js\nx\n```\n\n| a |\n|---|\n| 1 |";
  const parts = markdownBlocks(src);
  assert.deepEqual(parts.map((p) => p.match(/^<(\w+)/)[1]), ["h1", "p", "ul", "div", "table"]);
  assert.equal(parts.join(""), markdown(src));
  assert.deepEqual(markdownBlocks(""), []);
});

/* An element holding one node per inserted block, counting what is rebuilt. */
function fakeElement() {
  const el = {
    childNodes: [],
    built: 0,
    get lastChild() {
      const nodes = el.childNodes;
      return { remove: () => nodes.pop() };
    },
    set innerHTML(html) {
      el.childNodes = [];
      el.insertAdjacentHTML("beforeend", html);
    },
    insertAdjacentHTML(_, html) {
      const nodes = html.match(/<(h\d|p|ul|ol|div|table|blockquote|hr)\b/g) || [];
      el.built += nodes.length;
      el.childNodes.push(...nodes);
    },
  };
  return el;
}

test("drawBlocks rebuilds only the blocks from the first that changed", () => {
  const el = fakeElement();
  let shown = drawBlocks(el, [], markdownBlocks("# A\n\none\n\ntwo"));
  assert.equal(el.built, 3);
  shown = drawBlocks(el, shown, markdownBlocks("# A\n\none\n\ntwo three"));
  assert.equal(el.built, 4, "only the last paragraph is drawn again");
  shown = drawBlocks(el, shown, markdownBlocks("# A\n\none\n\ntwo three\n\nfour"));
  assert.equal(el.built, 5, "a new paragraph is drawn alone");
  assert.equal(drawBlocks(el, shown, shown), shown, "unchanged blocks draw nothing");
  assert.equal(el.built, 5);
  shown = drawBlocks(el, shown, markdownBlocks("# A"));
  assert.deepEqual(el.childNodes, ["<h1"]);
  assert.equal(el.built, 5, "dropping blocks only removes them");
});

test("drawBlocks draws everything again when the element was changed elsewhere", () => {
  const el = fakeElement();
  const shown = drawBlocks(el, [], markdownBlocks("one\n\ntwo"));
  el.childNodes.push("<p");
  const next = drawBlocks(el, shown, markdownBlocks("one\n\ntwo"));
  assert.equal(el.childNodes.length, 2);
  assert.equal(el.built, 4);
  assert.equal(next.length, 2);
});
