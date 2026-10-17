import { fuzzy } from "./fuzzy.js";

/* Go to file searches every path of a project, up to 200,000. Per listing it
   keeps the lowercased paths and the searches leading to the current query.
   A query that extends an earlier one can only match what that one matched,
   so each key typed scans fewer paths, and a key deleted finds its answer
   kept. Scores are fuzzy()'s, computed without allocating per path. */

// A search's answer is a sorted Float64Array of keys packing the score, the
// path's length and its place in the listing, so the sort compares numbers.
const SCORE = 2 ** 37;
const PLACE = 2 ** 21; // above the 200,000 paths a listing holds
const CAP = 0xffff;

const cache = new WeakMap(); // paths -> { lower, kept: [{ needle, keys }] }

/* searchFiles ranks the paths matching query, best first: a lower score,
   then a shorter path, then listing order. The answer reads rows on demand,
   so a caller draws the ones on screen and never copies the rest. */
export function searchFiles(paths, query) {
  const needle = query.trim().toLowerCase();
  if (!needle) return { length: paths.length, at: (rank) => ({ path: paths[rank], hits: null }) };
  let c = cache.get(paths);
  if (!c) cache.set(paths, (c = { lower: paths.map((p) => p.toLowerCase()), kept: [] }));
  while (c.kept.length && !needle.startsWith(c.kept.at(-1).needle)) c.kept.pop();
  let keys = c.kept.at(-1)?.needle === needle ? c.kept.at(-1).keys : null;
  if (!keys) {
    keys = rank(c.lower, [...needle], c.kept.at(-1)?.keys);
    c.kept.push({ needle, keys });
  }
  return {
    length: keys.length,
    at: (rank) => {
      const path = paths[keys[rank] % PLACE];
      return { path, hits: fuzzy(path, needle).hits };
    },
  };
}

/* rank scores the lowered paths, or only those in a previous answer, as
   fuzzy() would: its first hit counts twice plus every gap between hits,
   which comes to first + last - (hits - 1). */
function rank(lower, chars, from) {
  const n = from ? from.length : lower.length;
  const keys = new Float64Array(n);
  let count = 0;
  for (let k = 0; k < n; k++) {
    const i = from ? from[k] % PLACE : k;
    const hay = lower[i];
    const first = hay.indexOf(chars[0]);
    if (first < 0) continue;
    let at = first;
    for (let j = 1; j < chars.length && at >= 0; j++) at = hay.indexOf(chars[j], at + 1);
    if (at < 0) continue;
    const score = first + at - (chars.length - 1);
    keys[count++] = Math.min(score, CAP) * SCORE + Math.min(hay.length, CAP) * PLACE + i;
  }
  return keys.slice(0, count).sort();
}
