/* Syntax colouring for the file editor.

   Hand-written for the same reason as markdown.js: the escape function *is*
   the safety boundary. Every run of source text goes through esc() and the
   only tags emitted are the fixed spans below, so nothing in a project file
   can reach the DOM as markup.

   It is a lexer, not a parser — comments, strings, numbers and keywords —
   which is most of what colour buys you when reading code and costs one pass
   per line. Lines are memoised on the state they start in and their text, so
   a keystroke re-colours the line under the cursor and reuses every other. */

const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;" };

function esc(s) {
  return s.replace(/[&<>]/g, (c) => ESCAPES[c]);
}

function span(cls, text) {
  return `<span class="hl-${cls}">${esc(text)}</span>`;
}

/* ------------------------------------------------------------- languages */

const JS = "as async await break case catch class const continue debugger default delete do else export extends finally for from function get if import in instanceof let new of return set static super switch this throw try typeof var void while yield true false null undefined";

const KEYWORDS = {
  js: JS,
  ts: `${JS} abstract declare enum implements interface keyof namespace never readonly satisfies type unknown any boolean number string`,
  go: "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil iota append cap copy delete len make new panic recover bool byte error float32 float64 int int32 int64 rune string uint uint8",
  py: "and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield True False None self",
  rs: "as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return static struct super trait type unsafe use where while true false Some None Ok Err self Self",
  c: "auto break case catch char class const constexpr continue default delete do double else enum extern float for goto if inline int long namespace new private protected public register return short signed sizeof static struct switch template this throw try typedef typename union unsigned using virtual void volatile while true false null nullptr",
  sh: "case cd do done elif else esac exit export fi for function if in local return set source then unset until while",
  sql: "add all alter and as asc by create default delete desc distinct drop exists foreign from group having in index inner insert into is join key left limit not null offset on or order outer primary references right select set table union update values where",
  json: "true false null",
  yaml: "true false null yes no on off",
  css: "",
};

/* What starts a comment, what quotes a string, and which words are keywords.
   Anything unlisted takes the C-family defaults, which is why the table only
   holds the languages that differ. */
const SYNTAX = {
  py: { line: "#", block: null, quotes: `"'` },
  sh: { line: "#", block: null, quotes: `"'` },
  yaml: { line: "#", block: null, quotes: `"'` },
  sql: { line: "--", block: ["/*", "*/"], quotes: `"'` },
  json: { line: "", block: null, quotes: `"` },
  js: { line: "//", block: ["/*", "*/"], quotes: "\"'`" },
  ts: { line: "//", block: ["/*", "*/"], quotes: "\"'`" },
  go: { line: "//", block: ["/*", "*/"], quotes: "\"'`" },
};

const DEFAULT_SYNTAX = { line: "//", block: ["/*", "*/"], quotes: `"'` };

/* Memoised: a spec carries a Set, and building one per line would put the
   whole keyword table on the keystroke path. */
const SPECS = new Map();

function spec(lang) {
  let s = SPECS.get(lang);
  if (s) return s;
  const kw = KEYWORDS[lang];
  if (kw === undefined) return null;
  s = { ...(SYNTAX[lang] || DEFAULT_SYNTAX), words: new Set(kw ? kw.split(" ") : []) };
  SPECS.set(lang, s);
  return s;
}

/* The extension is the whole of the guess: reading the first line for a
   shebang buys one file type and costs a branch on every other. */
const BY_EXT = {
  js: "js", mjs: "js", cjs: "js", jsx: "js",
  ts: "ts", tsx: "ts", mts: "ts",
  go: "go", py: "py", pyi: "py", rs: "rs",
  c: "c", h: "c", cc: "c", cpp: "c", hpp: "c", cs: "c", java: "c", kt: "c", swift: "c",
  sh: "sh", bash: "sh", zsh: "sh", fish: "sh",
  sql: "sql", json: "json", jsonc: "json",
  yaml: "yaml", yml: "yaml", toml: "yaml", ini: "yaml", conf: "yaml", env: "yaml",
  css: "css", scss: "css", less: "css",
  html: "markup", htm: "markup", xml: "markup", svg: "markup", vue: "markup",
  md: "md", markdown: "md",
};

/* Files named rather than suffixed. */
const BY_NAME = {
  Makefile: "sh", Dockerfile: "sh", ".gitignore": "sh", ".env": "yaml",
};

export function langOf(path) {
  const name = String(path || "").split("/").pop();
  if (BY_NAME[name]) return BY_NAME[name];
  const dot = name.lastIndexOf(".");
  return (dot > 0 && BY_EXT[name.slice(dot + 1).toLowerCase()]) || "";
}

/* ------------------------------------------------------------------ code */

const RE_CACHE = new Map();

function scanner(lang, s) {
  let re = RE_CACHE.get(lang);
  if (re) return re;
  const parts = [];
  if (s.line) parts.push(`${escapeRe(s.line)}.*`);
  if (s.block) parts.push(escapeRe(s.block[0]));
  for (const q of s.quotes) parts.push(`${q}(?:\\\\.|[^\\\\${q}])*${q}?`);
  parts.push("\\b\\d(?:[\\w.]|[eE][+-])*");
  parts.push("[A-Za-z_$][\\w$]*");
  re = new RegExp(parts.join("|"), "g");
  RE_CACHE.set(lang, re);
  return re;
}

function escapeRe(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/* codeLine returns the state the next line starts in, which is only ever
   "block" — an unterminated string stops at the end of its line, the way a
   reader reads it. */
function codeLine(text, state, lang, s, out) {
  let i = 0;
  if (state === "block") {
    const end = text.indexOf(s.block[1]);
    if (end < 0) {
      out.push(span("c", text));
      return "block";
    }
    i = end + s.block[1].length;
    out.push(span("c", text.slice(0, i)));
  }

  const re = scanner(lang, s);
  re.lastIndex = i;
  let m;
  while ((m = re.exec(text))) {
    if (m.index > i) out.push(esc(text.slice(i, m.index)));
    const tok = m[0];
    i = m.index + tok.length;
    if (s.line && tok.startsWith(s.line)) {
      out.push(span("c", tok));
      return "";
    }
    if (s.block && tok === s.block[0]) {
      const end = text.indexOf(s.block[1], i);
      if (end < 0) {
        out.push(span("c", text.slice(m.index)));
        return "block";
      }
      i = end + s.block[1].length;
      out.push(span("c", text.slice(m.index, i)));
      re.lastIndex = i;
      continue;
    }
    if (s.quotes.includes(tok[0])) out.push(span("s", tok));
    else if (tok[0] >= "0" && tok[0] <= "9") out.push(span("n", tok));
    else if (s.words.has(tok)) out.push(span("k", tok));
    // A name before a bracket is being called, which is worth seeing.
    else if (text[i] === "(") out.push(span("f", tok));
    else out.push(esc(tok));
  }
  if (i < text.length) out.push(esc(text.slice(i)));
  return "";
}

/* ---------------------------------------------------------------- markup */

const TAG = /<!--|-->|<\/?[A-Za-z][\w:-]*|\/?>|[\w:-]+(?==)|"[^"]*"|'[^']*'/g;

/* markupLine also opens and closes the js and css states, so a .vue file's
   script and style blocks are coloured as what they are. */
function markupLine(text, state, out) {
  let i = 0;
  if (state === "comment") {
    const end = text.indexOf("-->");
    if (end < 0) {
      out.push(span("c", text));
      return "comment";
    }
    i = end + 3;
    out.push(span("c", text.slice(0, i)));
  }

  TAG.lastIndex = i;
  let m;
  let next = "";
  while ((m = TAG.exec(text))) {
    if (m.index > i) out.push(esc(text.slice(i, m.index)));
    const tok = m[0];
    i = m.index + tok.length;
    if (tok === "<!--") {
      const end = text.indexOf("-->", i);
      if (end < 0) {
        out.push(span("c", text.slice(m.index)));
        return "comment";
      }
      i = end + 3;
      out.push(span("c", text.slice(m.index, i)));
      TAG.lastIndex = i;
      continue;
    }
    if (tok[0] === "<") {
      out.push(span("t", tok));
      const name = tok.slice(tok[1] === "/" ? 2 : 1).toLowerCase();
      if (tok[1] !== "/" && (name === "script" || name === "style")) {
        next = name === "script" ? "js" : "css";
      } else if (tok[1] === "/" && (name === "script" || name === "style")) {
        next = "";
      }
    } else if (tok === ">" || tok === "/>") out.push(span("t", tok));
    else if (tok[0] === '"' || tok[0] === "'") out.push(span("s", tok));
    else out.push(span("a", tok));
  }
  if (i < text.length) out.push(esc(text.slice(i)));
  return next;
}

/* ------------------------------------------------------------- markdown */

/* No underscore emphasis: it would light up half of every snake_case name. */
const MD_INLINE = /`[^`]*`|\*\*[^*]+\*\*|\*[^*]+\*|\[[^\]]*\]\([^)]*\)/g;

function mdLine(text, state, out) {
  if (state === "fence") {
    out.push(span("s", text));
    return /^\s*(?:`{3,}|~{3,})\s*$/.test(text) ? "" : "fence";
  }
  if (/^\s*(?:`{3,}|~{3,})/.test(text)) {
    out.push(span("s", text));
    return "fence";
  }
  if (/^ {0,3}#{1,6}\s/.test(text)) {
    out.push(span("k", text));
    return "";
  }
  if (/^ {0,3}>/.test(text)) {
    out.push(span("c", text));
    return "";
  }
  const bullet = /^(\s*(?:[-*+]|\d{1,9}[.)])\s+)(.*)$/.exec(text);
  let rest = text;
  if (bullet) {
    out.push(span("t", bullet[1]));
    rest = bullet[2];
  }
  let i = 0;
  MD_INLINE.lastIndex = 0;
  let m;
  while ((m = MD_INLINE.exec(rest))) {
    if (m.index > i) out.push(esc(rest.slice(i, m.index)));
    out.push(span(m[0][0] === "`" ? "s" : m[0][0] === "[" ? "f" : "k", m[0]));
    i = m.index + m[0].length;
  }
  if (i < rest.length) out.push(esc(rest.slice(i)));
  return "";
}

/* ----------------------------------------------------------------- lines */

/* One entry per distinct (language, starting state, line). Typing only ever
   invalidates the line being typed on, so the cache is what keeps colouring a
   long file off the keystroke path. The cap is a memory bound, not a policy:
   overflowing simply starts again. */
const LINES = new Map();
const MAX_CACHED = 8000;

function lineHTML(lang, state, text) {
  const key = `${lang} ${state} ${text}`;
  const hit = LINES.get(key);
  if (hit) return hit;

  const out = [];
  let next;
  if (lang === "markup") {
    next = state.startsWith("js") || state.startsWith("css")
      ? codeInMarkup(text, state, out)
      : markupLine(text, state, out);
  } else if (lang === "md") {
    next = mdLine(text, state, out);
  } else {
    next = codeLine(text, state, lang, spec(lang), out);
  }

  const result = { html: out.join(""), next };
  if (LINES.size >= MAX_CACHED) LINES.clear();
  LINES.set(key, result);
  return result;
}

/* Inside a <script> or <style> block the line is code, and the closing tag
   ends the block wherever on the line it falls. The state carries the inner
   language and whether that language is mid-comment: "js", "js:block". */
function codeInMarkup(text, state, out) {
  const [lang, inner = ""] = state.split(":");
  const close = /<\/(script|style)/i.exec(text);
  if (!close) {
    const next = codeLine(text, inner, lang, spec(lang), out);
    return next === "block" ? `${lang}:block` : lang;
  }
  codeLine(text.slice(0, close.index), inner, lang, spec(lang), out);
  return markupLine(text.slice(close.index), "", out);
}

/* Per-line HTML lets the editor patch only changed lines. Lexer state still
   flows through every line, including when an earlier edit opens a comment. */
export function highlightLines(text, lang) {
  const lines = String(text ?? "").split("\n");
  const out = [];
  if (!lang) return lines.map(esc);
  let state = "";
  for (const line of lines) {
    const r = lineHTML(lang, state, line);
    out.push(r.html);
    state = r.next;
  }
  return out;
}

export function highlight(text, lang) {
  if (!lang) return esc(String(text ?? ""));
  return highlightLines(text, lang).join("\n");
}
