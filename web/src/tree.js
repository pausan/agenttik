/* The sidebar Tree pane. The server sends a flat list of paths; this
   turns it into folders and files and filters it as you type.

   Filtering matches the *whole* path, not the file name, so "wesst" finds
   web/src/store.js. The letters that matched are reported per node — a folder
   collecting the letters every match used inside it — so the tree can
   highlight them without knowing where in the path each node sits. */

import { fuzzy } from "./fuzzy.js";

/* buildTree groups paths into nodes. Folders come before files and both are
   sorted by name, which is the order a file manager shows. */
export function buildTree(paths) {
  const root = node("", "", true);
  for (const path of paths) {
    const parts = path.split("/").filter(Boolean);
    let at = root;
    for (let i = 0; i < parts.length; i++) {
      const dir = i < parts.length - 1;
      const full = parts.slice(0, i + 1).join("/");
      let next = at.index.get(parts[i]);
      if (!next) {
        next = node(parts[i], full, dir);
        at.index.set(parts[i], next);
        at.children.push(next);
      }
      at = next;
    }
  }
  return sortTree(root);
}

function node(name, path, dir) {
  return { name, path, dir, children: dir ? [] : null, index: dir ? new Map() : null, hits: null };
}

/* sortTree also drops the lookup maps, which were only needed while building
   and would otherwise be walked by Vue's reactivity for nothing. */
function sortTree(n) {
  n.index = null;
  if (!n.children) return n;
  n.children.sort((a, b) => b.dir - a.dir || a.name.localeCompare(b.name));
  for (const c of n.children) sortTree(c);
  return n;
}

/* filterTree keeps the files whose full path contains the filter's letters in
   order, and the folders that lead to one. An empty filter returns the tree
   unchanged, highlights cleared. */
export function filterTree(root, filter) {
  const needle = filter.trim();
  if (!needle) return clearHits(root);
  return walk(root, needle)?.node || { ...root, children: [] };
}

function walk(n, needle) {
  if (!n.dir) {
    const m = fuzzy(n.path, needle);
    if (!m) return null;
    return { node: { ...n, hits: own(n, m.hits) }, pathHits: m.hits || [] };
  }
  const children = [];
  const hits = new Set();
  for (const child of n.children) {
    const kept = walk(child, needle);
    if (!kept) continue;
    children.push(kept.node);
    for (const h of kept.pathHits) hits.add(h);
  }
  if (!children.length && n.path) return null;
  const pathHits = [...hits].sort((a, b) => a - b);
  return { node: { ...n, children, hits: own(n, pathHits) }, pathHits };
}

/* own narrows hits down to the node's own name. Hits index into the path of
   whichever file matched, so a folder keeps only the ones that land inside the
   slice its own name occupies — its name is the tail of its own path, and
   every path underneath shares that prefix. */
function own(n, hits) {
  if (!hits || !hits.length || !n.name) return null;
  const start = n.path.length - n.name.length;
  const mine = [];
  for (const h of hits) if (h >= start && h < n.path.length) mine.push(h - start);
  return mine.length ? mine : null;
}

function clearHits(n) {
  const copy = { ...n, hits: null };
  if (n.children) copy.children = n.children.map(clearHits);
  return copy;
}

/* countFiles is what the pane shows next to the filter. */
export function countFiles(n) {
  if (!n.dir) return 1;
  let total = 0;
  for (const c of n.children) total += countFiles(c);
  return total;
}
