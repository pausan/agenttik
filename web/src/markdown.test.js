import { test } from "node:test";
import assert from "node:assert/strict";

import { markdown } from "./markdown.js";

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
    "<pre><code>&lt;script&gt;bad()&lt;/script&gt;</code></pre>");
});

test("a fenced block keeps its text and names its language", () => {
  assert.equal(markdown("```go\nfunc main() {}\n```"),
    '<pre><code class="language-go">func main() {}</code></pre>');
});

test("an unclosed fence still renders", () => {
  assert.equal(markdown("```\nstill here"), "<pre><code>still here</code></pre>");
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
