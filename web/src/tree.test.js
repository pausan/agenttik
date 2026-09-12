import { test } from "node:test";
import assert from "node:assert/strict";

import { buildTree, countFiles, filterTree } from "./tree.js";

const PATHS = [
  "web/src/store.js",
  "web/src/api.js",
  "web/package.json",
  "app/internal/store/store.go",
  "README.md",
];

/* shape renders a tree as indented lines, which is what the pane draws. */
function shape(n, depth = -1) {
  const here = depth < 0 ? [] : ["  ".repeat(depth) + n.name + (n.dir ? "/" : "")];
  return here.concat((n.children || []).flatMap((c) => shape(c, depth + 1)));
}

test("folders come before files and both groups are sorted by name", () => {
  assert.deepEqual(shape(buildTree(PATHS)), [
    "app/",
    "  internal/",
    "    store/",
    "      store.go",
    "web/",
    "  src/",
    "    api.js",
    "    store.js",
    "  package.json",
    "README.md",
  ]);
});

test("a folder is one node however many files sit under it", () => {
  const tree = buildTree(PATHS);
  assert.equal(countFiles(tree), PATHS.length);
  assert.equal(tree.children.filter((c) => c.name === "web").length, 1);
});

test("an empty filter returns the whole tree with no highlights", () => {
  const tree = filterTree(buildTree(PATHS), "  ");
  assert.equal(countFiles(tree), PATHS.length);
  assert.equal(tree.children[0].hits, null);
});

test("the filter matches letters spread across the whole path", () => {
  const tree = filterTree(buildTree(PATHS), "wesst");
  assert.deepEqual(shape(tree), ["web/", "  src/", "    store.js"]);
});

test("folders with nothing left are dropped", () => {
  const tree = filterTree(buildTree(PATHS), "readme");
  assert.deepEqual(shape(tree), ["README.md"]);
});

test("hits are reported against each node's own name", () => {
  const tree = filterTree(buildTree(PATHS), "wesst");
  const web = tree.children[0];
  const src = web.children[0];
  const file = src.children[0];
  // "wesst" matches web/src/store.js at w, e, s, s, t. Each node keeps only
  // the letters that fall inside its own name.
  assert.deepEqual(web.hits, [0, 1]); //   "we" of web
  assert.deepEqual(src.hits, [0]); //      "s" of src
  assert.deepEqual(file.hits, [0, 1]); //  "st" of store.js
  assert.equal(web.hits.map((h) => web.name[h]).join(""), "we");
  assert.equal(file.hits.map((h) => file.name[h]).join(""), "st");
});

test("a folder collects the letters every match under it used", () => {
  const tree = filterTree(buildTree(PATHS), "js");
  const web = tree.children[0];
  // Both web/src files and web/package.json match, through different letters.
  assert.deepEqual(shape(tree), ["web/", "  src/", "    api.js", "    store.js", "  package.json"]);
  assert.equal(web.hits, null); // no letter of "js" lands in "web" itself
});

test("no match at all leaves an empty tree", () => {
  const tree = filterTree(buildTree(PATHS), "zzz");
  assert.deepEqual(shape(tree), []);
});

test("a folder name on its own keeps everything under it", () => {
  const tree = filterTree(buildTree(PATHS), "internal");
  assert.deepEqual(shape(tree), ["app/", "  internal/", "    store/", "      store.go"]);
});


test("ignored files stay visible and folders still come first", () => {
  const ignored = ["alpha.txt", "middle/b.txt", "middle/cache/item.txt"];
  const tree = buildTree(
    ["zebra.txt", "middle/z.txt", ...ignored, "middle/a.txt"],
    ignored,
    ["beta"],
  );
  assert.deepEqual(shape(tree), [
    "beta/", "middle/", "  cache/", "    item.txt", "  a.txt", "  b.txt",
    "  z.txt", "alpha.txt", "zebra.txt",
  ]);
  const middle = tree.children[1];
  assert.equal(middle.ig, false);
  assert.equal(middle.children[0].ig, true);
  assert.equal(middle.children[1].ig, false);
  assert.equal(middle.children[2].ig, true);
  const filtered = filterTree(tree, "midb.txt");
  assert.deepEqual(shape(filtered), ["middle/", "  b.txt"]);
  assert.equal(filtered.children[0].children[0].ig, true);
});
