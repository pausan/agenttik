/* A unified diff into rows, and those rows into two columns.

   Both views come from one parse: the unified list is what git printed, with
   the line numbers the hunk headers imply, and the split list pairs each run
   of removals with the run of additions that replaced it. */

/* A diff is coloured a line at a time, so it is worth capping: a whole
   rewritten file is thousands of lines nobody reads to the end. */
export const MAX_LINES = 4000;

/* The headers git writes around each file. They carry no line of their own,
   so the split view drops them and the unified view dims them. */
const META = /^(?:diff |index |--- |\+\+\+ |old mode|new mode|new file|deleted file|similarity|rename |Binary files|\\)/;

const HUNK = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

/* parseDiff numbers the rows as it goes: a hunk header says where both sides
   resume, and every row after it advances the side or sides it belongs to. */
export function parseDiff(src, max = MAX_LINES) {
  const rows = [];
  let truncated = false;
  let left = 0;
  let right = 0;

  for (const text of String(src || "").replace(/\n$/, "").split("\n")) {
    if (rows.length >= max) {
      truncated = true;
      break;
    }
    const hunk = HUNK.exec(text);
    if (hunk) {
      left = Number(hunk[1]);
      right = Number(hunk[2]);
      rows.push({ kind: "hunk", text });
    } else if (text.startsWith("@@") || META.test(text)) {
      rows.push({ kind: "meta", text });
    } else if (text.startsWith("+")) {
      rows.push({ kind: "add", text, body: text.slice(1), right: right++ });
    } else if (text.startsWith("-")) {
      rows.push({ kind: "del", text, body: text.slice(1), left: left++ });
    } else {
      // Context, including the empty last line git leaves on an unterminated
      // file; both sides advance.
      rows.push({ kind: "ctx", text, body: text.slice(1), left: left++, right: right++ });
    }
  }
  return { rows, truncated, empty: rows.length === 0 };
}

/* pair puts the rows in two columns. A run of removals lines up with the run
   of additions that follows it, one against one, and whichever run is shorter
   leaves blank cells opposite the rest. The +/- markers go: which column a
   line is in already says which side it belongs to. */
export function pair(rows) {
  const out = [];
  for (let i = 0; i < rows.length; ) {
    const row = rows[i];
    if (row.kind === "meta") {
      i++;
      continue;
    }
    if (row.kind === "hunk") {
      out.push({ kind: "hunk", text: row.text });
      i++;
      continue;
    }
    if (row.kind === "ctx") {
      const cell = { n: 0, text: row.body };
      out.push({ kind: "ctx", l: { ...cell, n: row.left }, r: { ...cell, n: row.right } });
      i++;
      continue;
    }
    const dels = [];
    const adds = [];
    while (i < rows.length && rows[i].kind === "del") dels.push(rows[i++]);
    while (i < rows.length && rows[i].kind === "add") adds.push(rows[i++]);
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) {
      out.push({
        kind: "chg",
        l: dels[k] ? { n: dels[k].left, text: dels[k].body } : null,
        r: adds[k] ? { n: adds[k].right, text: adds[k].body } : null,
      });
    }
  }
  return out;
}

/* The colour of one unified row. */
export function classOf(kind) {
  if (kind === "hunk") return "text-primary";
  if (kind === "add") return "text-success";
  if (kind === "del") return "text-error";
  if (kind === "meta") return "text-dimmed";
  return "text-muted";
}
