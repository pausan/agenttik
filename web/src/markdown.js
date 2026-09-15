import { apiURL } from "./api.js";

/* Markdown for the transcript. Agent replies are written in it, and reading
   raw ** and ``` is worse than reading the text.

   Hand-written rather than marked plus a sanitiser, for one reason: here the
   escaping *is* the parser. Every run of source text goes through esc() and
   the only tags emitted are the fixed list below, so no sanitising pass is
   needed to know that nothing an agent writes can inject HTML. It costs about
   200 lines and no dependency, and the ranking-style bugs a small parser
   invites are covered by markdown.test.js.

   Supported: fenced code, headings, rules, blockquotes, pipe tables, nested
   ordered and unordered lists, paragraphs, and inline code, bold, italic,
   strikethrough, links, and paths, which open the file rather than a page.
   Anything else renders as its own text. */

const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" };

function esc(s) {
  return s.replace(/[&<>"]/g, (c) => ESCAPES[c]);
}

export function markdown(src) {
  if (!src) return "";
  return blocks(String(src).replace(/\r\n?/g, "\n").split("\n")).join("");
}

/* ------------------------------------------------------------------ blocks */

const FENCE = /^ {0,3}(`{3,}|~{3,})\s*([^\s`]*)/;
const HEADING = /^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const RULE = /^ {0,3}([-*_])\s*(?:\1\s*){2,}$/;
const QUOTE = /^ {0,3}>/;
const BULLET = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/;
const ORDERED = /^\d/;
const TABLE_RULE = /^\s*\|?(?:\s*:?-+:?\s*\|)+\s*:?-*:?\s*\|?\s*$/;

function blocks(lines) {
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }

    const fence = FENCE.exec(line);
    if (fence) {
      const close = new RegExp(`^ {0,3}\\${fence[1][0]}{${fence[1].length},}\\s*$`);
      const body = [];
      for (i++; i < lines.length && !close.test(lines[i]); i++) body.push(lines[i]);
      i++; // the closing fence, or past the end when it never came
      out.push(codeBlock(body.join("\n"), fence[2]));
      continue;
    }

    const heading = HEADING.exec(line);
    if (heading) {
      const n = heading[1].length;
      out.push(`<h${n}>${inline(heading[2])}</h${n}>`);
      i++;
      continue;
    }

    if (RULE.test(line)) {
      out.push("<hr>");
      i++;
      continue;
    }

    if (QUOTE.test(line)) {
      const body = [];
      while (
        i < lines.length &&
        (QUOTE.test(lines[i]) || (body.length && lines[i].trim() && !startsBlock(lines, i)))
      ) {
        body.push(lines[i++].replace(/^ {0,3}>\s?/, ""));
      }
      out.push("<blockquote>" + blocks(body).join("") + "</blockquote>");
      continue;
    }

    if (line.includes("|") && TABLE_RULE.test(lines[i + 1] || "")) {
      const [html, next] = table(lines, i);
      out.push(html);
      i = next;
      continue;
    }

    if (BULLET.test(line)) {
      const [html, next] = list(lines, i);
      out.push(html);
      i = next;
      continue;
    }

    const body = [];
    while (i < lines.length && lines[i].trim() && !startsBlock(lines, i)) body.push(lines[i++]);
    // A line that both continues the paragraph and opens a block would loop
    // forever; taking it as prose is the safe way out.
    if (!body.length) body.push(lines[i++]);
    out.push("<p>" + inline(body.join("\n")) + "</p>");
  }
  return out;
}

/* startsBlock is what interrupts a paragraph without a blank line first. */
function startsBlock(lines, i) {
  const line = lines[i];
  return (
    FENCE.test(line) ||
    HEADING.test(line) ||
    RULE.test(line) ||
    QUOTE.test(line) ||
    BULLET.test(line) ||
    (line.includes("|") && TABLE_RULE.test(lines[i + 1] || ""))
  );
}

function codeBlock(body, lang) {
  // The class is only emitted for a plain word, so lang needs no escaping.
  const cls = /^[\w+#.-]+$/.test(lang) ? ` class="language-${lang}"` : "";
  return `<pre><code${cls}>${esc(body)}</code></pre>`;
}

/* list collects one list and returns it with the line to carry on from.
   Indentation decides nesting: a bullet indented past its parent belongs to
   the item above it, and that item is then parsed as blocks of its own. */
function list(lines, start) {
  const first = BULLET.exec(lines[start]);
  const indent = first[1].length;
  const ordered = ORDERED.test(first[2]);
  const items = [];
  let i = start;

  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      // A blank line stays in the list only while the list carries on.
      const next = lines[i + 1] || "";
      const m = BULLET.exec(next);
      if (!((m && m[1].length >= indent) || /^\s{2,}\S/.test(next))) break;
      items[items.length - 1]?.push("");
      i++;
      continue;
    }
    const m = BULLET.exec(line);
    if (m && m[1].length <= indent + 1) {
      if (ORDERED.test(m[2]) !== ordered) break; // a list of the other kind
      items.push([m[3]]);
      i++;
      continue;
    }
    if (!items.length) break;
    // A nested bullet or an indented line continues the item above.
    items[items.length - 1].push(m || /^\s/.test(line) ? dedent(line, indent + 2) : line.trim());
    i++;
  }

  const tag = ordered ? "ol" : "ul";
  const body = items
    .map((item) => {
      const own = trimBlank(item);
      return "<li>" + (own.length === 1 ? inline(own[0]) : blocks(own).join("")) + "</li>";
    })
    .join("");
  return [`<${tag}>${body}</${tag}>`, i];
}

function dedent(line, n) {
  return /^\s/.test(line.slice(0, n)) ? line.slice(n) : line.trimStart();
}

function trimBlank(lines) {
  let a = 0;
  let b = lines.length;
  while (a < b && !lines[a].trim()) a++;
  while (b > a && !lines[b - 1].trim()) b--;
  return lines.slice(a, b);
}

function table(lines, start) {
  const head = cells(lines[start]);
  const rows = [];
  let i = start + 2;
  while (i < lines.length && lines[i].trim() && lines[i].includes("|")) rows.push(cells(lines[i++]));
  const th = head.map((c) => `<th>${inline(c)}</th>`).join("");
  const body = rows
    .map((r) => "<tr>" + r.map((c) => `<td>${inline(c)}</td>`).join("") + "</tr>")
    .join("");
  return [`<table><thead><tr>${th}</tr></thead><tbody>${body}</tbody></table>`, i];
}

function cells(line) {
  return line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
}

/* ------------------------------------------------------------------ inline */

/* One alternation, so a code span wins over any marker inside it. The italic
   branches carry the character in front of the marker and put it back, which
   keeps snake_case and a*b from turning into emphasis without needing a
   lookbehind. */
const INLINE = new RegExp(
  [
    "(`+)([\\s\\S]*?)\\1", //                        1,2  code span
    "\\*\\*([\\s\\S]+?)\\*\\*", //                   3    bold
    "__([\\s\\S]+?)__", //                           4    bold
    "(^|[^\\w*])\\*([^*\\n]+)\\*(?!\\w)", //         5,6  italic
    "(^|[^\\w_])_([^_\\n]+)_(?!\\w)", //             7,8  italic
    "~~([\\s\\S]+?)~~", //                           9    strikethrough
    "!?\\[([^\\]]*)\\]\\((<[^>]*>|[^)\\s]*)[^)]*\\)", //     10,11 link or image
    "<(https?://[^>\\s]+)>", //                      12   autolink
    "(https?://[^\\s<>\\[\\]`]+)", //                13   bare url
  ].join("|"),
  "g",
);

function inline(src) {
  let out = "";
  let at = 0;
  INLINE.lastIndex = 0;
  let m;
  while ((m = INLINE.exec(src))) {
    out += esc(src.slice(at, m.index));
    at = m.index + m[0].length;
    if (m[2] !== undefined) {
      const text = m[2].replace(/^ (.*) $/, "$1");
      const code = "<code>" + esc(text) + "</code>";
      out += fileLink(text, code) || code;
    } else if (m[3] !== undefined || m[4] !== undefined) {
      out += "<strong>" + inline(m[3] ?? m[4]) + "</strong>";
    } else if (m[6] !== undefined) {
      out += m[5] + "<em>" + inline(m[6]) + "</em>";
    } else if (m[8] !== undefined) {
      out += m[7] + "<em>" + inline(m[8]) + "</em>";
    } else if (m[9] !== undefined) {
      out += "<del>" + inline(m[9]) + "</del>";
    } else if (m[11] !== undefined) {
      // An image is rendered as its link: the transcript fetches nothing.
      out += link(m[11], inline(m[10] || m[11]));
    } else if (m[12] !== undefined) {
      out += link(m[12], esc(m[12]));
    } else {
      // Trailing punctuation reads as the sentence's, not the url's.
      const url = m[13].replace(/[.,;:!?)\]]+$/, "");
      at = m.index + url.length;
      out += link(url, esc(url));
    }
    // The recursive calls above share this regex, so resume deliberately.
    INLINE.lastIndex = at;
  }
  return (out + esc(src.slice(at))).replace(/\n/g, "<br>");
}

/* link keeps the schemes that can be handed to the browser and opens them in
   it, rather than navigating the app away. Anything else is offered to
   fileLink: an agent writing [store.js](web/src/store.js) means the file, and
   an absolute path would otherwise become a link to our own origin. A scheme
   that could do something when clicked matches neither and stays as text. */
function link(url, text) {
  const href = url.trim().replace(/^<(.*)>$/, "$1");
  if (/^(?:https?:|mailto:|#)/i.test(href) || /^\/api\/attachments\/[a-f0-9]{64}\.(png|jpg|gif|webp)$/.test(href)) {
    return `<a href="${esc(apiURL(href))}" target="_blank" rel="noreferrer noopener">${text}</a>`;
  }
  return fileLink(href, text, true) || text;
}

/* ------------------------------------------------------------ file links */

/* What a coding agent writes when it points at a file: `web/src/store.js:801`
   in a code span, far more often than a markdown link. Both open the file in
   a tab, which is the transcript's own business rather than the browser's —
   so this is a button carrying the path in a data attribute, not an anchor
   that would navigate somewhere, and MarkdownContent.vue answers the click.

   An extension is what tells a path from the other things a code span holds:
   `S.detail` and `account/rateLimits/read` are not files. */
const FILE_EXT =
  /\.(?:js|mjs|cjs|jsx|ts|tsx|vue|svelte|go|mod|sum|rs|py|rb|php|java|kt|swift|c|h|cc|cpp|hpp|cs|sh|bash|fish|zsh|sql|css|scss|html?|xml|json|ya?ml|toml|ini|conf|env|lock|md|markdown|txt|csv|svg|png|jpe?g|gif|webp)$/i;

/* An optional file:// prefix, the path, and the line it named — as `:801`,
   or `#L801` the way a forge writes it. A trailing column is dropped. */
const FILE_REF = /^(?:file:\/\/)?(\/?[\w.@~+-]+(?:\/[\w.@~+-]+)*)(?::(\d+)(?::\d+)?|#L?(\d+))?$/;

function fileRef(target, explicit) {
  if (explicit) {
    try { target = decodeURIComponent(target); } catch { return null; }
    target = target.replace(/^file:\/\//i, "");
    const m = /^(.*?)(?::(\d+)(?::\d+)?|#L?(\d+)|#[^#]*)?$/.exec(target);
    if (!m || !m[1] || /[:?#\\\x00-\x1f]/.test(m[1]) || m[1].startsWith("//")) return null;
    return { path: m[1], line: m[2] || m[3] || "" };
  }
  const m = FILE_REF.exec(target.trim());
  if (!m || !FILE_EXT.test(m[1])) return null;
  return { path: m[1], line: m[2] || m[3] || "" };
}

/* fileLink returns null when the target is not a path, so a caller that has
   something else — a code span, a link to nowhere — renders it as it was. */
function fileLink(target, inner, explicit = false) {
  const ref = fileRef(target, explicit);
  if (!ref) return null;
  const label = ref.line ? `${ref.path}:${ref.line}` : ref.path;
  const line = ref.line ? ` data-line="${ref.line}"` : "";
  return `<button type="button" class="file" data-file="${esc(ref.path)}"${line} title="Open ${esc(label)}">${inner}</button>`;
}
