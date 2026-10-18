/* Go to file's search, off the window's thread. The window posts a listing,
   its paths joined by NULs, then queries; only the newest query waiting is
   answered, so a burst of keys costs one search. Ranked places travel back
   without a copy. */
import { fileSearch } from "./file-search.js";

let search = null;
let waiting = null;

onmessage = ({ data }) => {
  if (data.listing !== undefined) {
    search = fileSearch(data.listing ? data.listing.split("\0") : [], data.ignoredFrom);
    return;
  }
  if (!waiting) setTimeout(answer);
  waiting = data;
};

function answer() {
  const { id, query, all } = waiting;
  waiting = null;
  const { length, ranked } = search.rank(query, all);
  // The search keeps its own copy for the next key.
  const copy = ranked && ranked.slice();
  postMessage({ id, length, ranked: copy }, copy ? [copy.buffer] : []);
}
