/* The sidebar Tree pane. The server sends a flat list of paths; this
   turns it into folders and files and filters it as you type.

   Filtering matches the *whole* path, not the file name, so "wesst" finds
   web/src/store.js. The letters that matched are reported per node — a folder
   collecting the letters every match used inside it — so the tree can
   highlight them without knowing where in the path each node sits. */

import { fuzzy } from "./fuzzy.js";

/* buildTree groups paths into nodes. What git ignores goes in the tree too and
   is marked `ig`: the pane greys those and keeps their folders shut, so
   generated files are visible without being in the way.

   Folders come before files and both are sorted by name, which is the order a
   file manager shows — except that anything ignored sinks below everything
   that is not, so a folder reads as its own contents first and what was
   generated into it after. */
export function buildTree(paths, ignored = []) {
  const skip = ignored.length ? new Set(ignored) : null;
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
    if (skip?.has(path)) at.ig = true;
  }
  return sortTree(root);
}

function node(name, path, dir) {
  return {
    name,
    path,
    dir,
    children: dir ? [] : null,
    index: dir ? new Map() : null,
    hits: null,
    ig: false,
  };
}

/* sortTree also drops the lookup maps, which were only needed while building
   and would otherwise be walked by Vue's reactivity for nothing.

   Children are sorted after they have been walked, because a folder counts as
   ignored only once its own are known: git ignores files, so a folder is
   generated exactly when everything inside it is. */
function sortTree(n) {
  n.index = null;
  if (!n.children) return n;
  for (const c of n.children) sortTree(c);
  n.ig = n.children.length > 0 && n.children.every((c) => c.ig);
  n.children.sort((a, b) => a.ig - b.ig || b.dir - a.dir || a.name.localeCompare(b.name));
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
