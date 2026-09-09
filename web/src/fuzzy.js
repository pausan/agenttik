/* Fuzzy matching for the folder picker: what is typed after the last "/"
   never reaches the server, it narrows the listing we already have. */

/* fuzzy scores a subsequence match: hits early in the name and close together
   score lower, and lower wins. Null means the name does not match. */
export function fuzzy(name, needle) {
  const hay = name.toLowerCase();
  const hits = [];
  let from = 0;
  for (const ch of needle.toLowerCase()) {
    const at = hay.indexOf(ch, from);
    if (at < 0) return null;
    hits.push(at);
    from = at + 1;
  }
  let score = hits[0] * 2; // a match at the start beats one in the middle
  for (let i = 1; i < hits.length; i++) score += hits[i] - hits[i - 1] - 1;
  // Lowercasing can change a name's length (rare), and then the indexes are
  // no good for highlighting. Keep the match, drop the highlight.
  return { score, hits: hay.length === name.length ? hits : null };
}

/* filterDirs keeps the folders whose name contains the filter's letters in
   order, best match first. */
export function filterDirs(dirs, filter) {
  if (!filter) return dirs.map((dir) => ({ dir, hits: null }));
  const out = [];
  for (const dir of dirs) {
    const m = fuzzy(dir.name, filter);
    if (m) out.push({ dir, hits: m.hits, score: m.score, len: dir.name.length });
  }
  out.sort(
    (a, b) => a.score - b.score || a.len - b.len || a.dir.name.localeCompare(b.dir.name),
  );
  return out;
}

/* segments splits a name into the matched letters and the rest, so the
   highlight can be rendered without touching innerHTML. */
export function segments(name, hits) {
  if (!hits || !hits.length) return [{ text: name, hit: false }];
  const out = [];
  let at = 0;
  for (const h of hits) {
    if (h > at) out.push({ text: name.slice(at, h), hit: false });
    out.push({ text: name[h], hit: true });
    at = h + 1;
  }
  if (at < name.length) out.push({ text: name.slice(at), hit: false });
  return out;
}

/* fuzzyAny scores the best of a row's fields, so a filter can search a label
   and its keys without the two running together into one long string that
   almost anything matches. Null means the row does not match; an empty
   filter matches everything. */
export function fuzzyAny(fields, needle) {
  if (!needle) return 0;
  let best = null;
  for (const field of fields) {
    const m = field && fuzzy(field, needle);
    if (m && (best === null || m.score < best)) best = m.score;
  }
  return best;
}
