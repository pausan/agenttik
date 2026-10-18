/* Go to file searches every path of a project, up to 200,000. It runs in a
   worker (file-search-worker.js), as a short query over a large listing can
   cost tens of milliseconds, and typing must not wait for it.

   A search keeps the lowercased paths and the answers for the prefixes of
   the current query. A query that extends an earlier one can only match what
   that one matched, so each key typed scans fewer paths, and a key deleted
   finds its answer kept. Answers stay in listing order, so a rescan reads
   memory in order. Only the best TOP are ranked, the rest when asked. */

// Scores, after fzf: each letter matched earns MATCH and a bonus for where it
// lands. The first letter's bonus counts twice; a letter right after the one
// before keeps the bonus of the run's first letter; skipping letters costs
// GAP_START plus GAP_EXTEND per further letter. Two placements are scored,
// the better kept: the leftmost, and the rightmost, which finds the file's
// own name, each pulled tight from its far end as fzf's v1 does. Scoring
// every placement, as fzf's v2 does, cost twice as much over 200,000 paths.
const MATCH = 16;
const GAP_START = 3;
const GAP_EXTEND = 1;
const AT_SEGMENT = 9; // the path's start, or after "/"
const AT_WORD = 8; // after "-", "_", "." or a space
const AT_HUMP = 7; // a capital after a lowercase letter, a digit after a non-digit
const IN_RUN = 4; // the least a letter right after the one before earns
const IN_NAME = 2; // a letter in the file's own name, past the last "/"
const FIRST = 2;
const IGNORED = MATCH; // what a file git ignores loses, so project files lead

// An answer's keys pack the inverted score, the path's length and its place
// in the listing into one number, so ranking compares numbers: a higher
// score, then a shorter path, then listing order.
const SCORE = 2 ** 37;
const PLACE = 2 ** 21; // above the 200,000 paths a listing holds
const CAP = 0xffff;
const ZERO = 0x8000; // score 0 packs here, keeping negative scores positive
const TOP = 100; // ranked on a search; the rest wait for a scroll past them

/* fileSearch prepares a listing whose paths from ignoredFrom on are ignored
   by git. Its rank answers how many paths match a query and the places of the
   best in order: the best TOP, or all of them when all is set. An empty query
   answers ranked null: listing order. */
export function fileSearch(paths, ignoredFrom = paths.length) {
  const lower = paths.map((p) => p.toLowerCase());
  // Where each file's own name starts, past its last "/".
  const names = Uint16Array.from(lower, (p) => Math.min(p.lastIndexOf("/") + 1, CAP));
  const kept = [];
  return {
    rank(query, all = false) {
      const needle = query.trim().toLowerCase();
      if (!needle) return { length: paths.length, ranked: null };
      while (kept.length && !needle.startsWith(kept.at(-1).needle)) kept.pop();
      let found = kept.at(-1);
      if (found?.needle !== needle) {
        found = { needle, keys: scan(paths, lower, names, ignoredFrom, [...needle], found?.keys), ranked: null };
        kept.push(found);
      }
      const { keys } = found;
      if (!found.ranked || (all && found.ranked.length < keys.length)) {
        found.ranked = places(!all && keys.length > TOP ? smallest(keys, TOP) : keys.slice().sort());
      }
      return { length: keys.length, ranked: found.ranked };
    },
  };
}

/* matchHits is where query's letters land in path, for highlighting, or
   null when they do not all match or lowercasing moved them. */
export function matchHits(path, query) {
  const lower = path.toLowerCase();
  const hits = [];
  const name = lower.lastIndexOf("/") + 1;
  if (lower.length !== path.length || align(lower, path, name, [...query.trim().toLowerCase()], hits) === null) return null;
  return hits;
}

/* scan scores every path, or only those of an earlier answer, in listing
   order. */
function scan(paths, lower, names, ignoredFrom, chars, from) {
  const n = from ? from.length : lower.length;
  const keys = new Float64Array(n);
  let count = 0;
  for (let k = 0; k < n; k++) {
    const i = from ? from[k] % PLACE : k;
    let score = align(lower[i], paths[i], names[i], chars, null);
    if (score === null) continue;
    if (i >= ignoredFrom) score -= IGNORED;
    const packed = Math.max(0, Math.min(ZERO - score, CAP));
    keys[count++] = packed * SCORE + Math.min(lower[i].length, CAP) * PLACE + i;
  }
  return keys.slice(0, count);
}

function places(sorted) {
  const out = new Int32Array(sorted.length);
  for (let k = 0; k < sorted.length; k++) out[k] = sorted[k] % PLACE;
  return out;
}

/* smallest picks the k smallest keys through a max-heap, sorted: most keys
   lose to its top in one comparison. */
function smallest(keys, k) {
  const heap = keys.slice(0, k);
  for (let i = (k >> 1) - 1; i >= 0; i--) sink(heap, i, heap[i]);
  for (let i = k; i < keys.length; i++) if (keys[i] < heap[0]) sink(heap, 0, keys[i]);
  return heap.sort();
}

function sink(heap, at, value) {
  const n = heap.length;
  for (;;) {
    let child = 2 * at + 1;
    if (child >= n) break;
    if (child + 1 < n && heap[child + 1] > heap[child]) child++;
    if (heap[child] <= value) break;
    heap[at] = heap[child];
    at = child;
  }
  heap[at] = value;
}

// The places of the two placements align weighs, grown as needed.
let LEFT = new Int32Array(64);
let RIGHT = new Int32Array(64);

/* align scores where chars best land in a path, or null when they are not all
   in it in order. lower is the path lowercased and name where its file's
   own name starts. hits, when given, receives the places of the placement
   scored. */
function align(lower, path, name, chars, hits) {
  const m = chars.length;
  if (LEFT.length < m) (LEFT = new Int32Array(2 * m)), (RIGHT = new Int32Array(2 * m));
  // Leftmost: each letter at its first place after the one before, then
  // pulled back from the last letter so the gaps close up.
  let at = -1;
  for (let j = 0; j < m; j++) {
    at = lower.indexOf(chars[j], at + 1);
    if (at < 0) return null;
  }
  for (let j = m - 1; j >= 0; j--) {
    LEFT[j] = at;
    if (j) at = lower.lastIndexOf(chars[j - 1], at - 1);
  }
  const left = score(path, LEFT, m, name);
  let right = left;
  // Rightmost: each letter at its last place before the one after, then
  // pulled forward from the first letter. When the leftmost already lies in
  // the file's name, so does this one, and there it rarely does better.
  if (LEFT[0] < name) {
    at = lower.length;
    for (let j = m - 1; j >= 0; j--) at = lower.lastIndexOf(chars[j], at - 1);
    for (let j = 0; j < m; j++) {
      RIGHT[j] = at;
      if (j < m - 1) at = lower.indexOf(chars[j + 1], at + 1);
    }
    if (RIGHT[0] !== LEFT[0]) right = score(path, RIGHT, m, name);
  }
  const best = right > left ? RIGHT : LEFT;
  if (hits) for (let j = 0; j < m; j++) hits.push(best[j]);
  return Math.max(left, right);
}

/* score adds up what each letter earns at its place. */
function score(path, places, m, name) {
  let run = bonus(path, places[0], name);
  let total = MATCH + FIRST * run;
  for (let j = 1; j < m; j++) {
    const at = places[j];
    const b = bonus(path, at, name);
    const gap = at - places[j - 1] - 1;
    if (gap === 0) {
      total += MATCH + Math.max(b, run, IN_RUN);
      if (b > run) run = b;
    } else {
      total += MATCH + b - GAP_START - GAP_EXTEND * (gap - 1);
      run = b;
    }
  }
  return total;
}

/* bonus is what a letter earns for landing at i: at the start of a folder or
   file name, a word, or a camelCase hump, plus a little inside the name. */
function bonus(path, i, name) {
  const inName = i >= name ? IN_NAME : 0;
  if (i === 0) return AT_SEGMENT + inName;
  const prev = path.charCodeAt(i - 1);
  if (prev === 47) return AT_SEGMENT + inName; // "/"
  if (prev === 45 || prev === 95 || prev === 46 || prev === 32) return AT_WORD + inName; // - _ . space
  const c = path.charCodeAt(i);
  if (c >= 65 && c <= 90 && prev >= 97 && prev <= 122) return AT_HUMP + inName;
  if (c >= 48 && c <= 57 && !(prev >= 48 && prev <= 57)) return AT_HUMP + inName;
  return inName;
}
