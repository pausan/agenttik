import { test } from "node:test";
import { deepStrictEqual, strictEqual } from "node:assert";
import { fileSearch, matchHits } from "./file-search.js";

const top = (paths, query, n = 5, ignoredFrom) => {
  const { ranked } = fileSearch(paths, ignoredFrom).rank(query, true);
  return [...ranked.slice(0, n)].map((i) => paths[i]);
};

test("file search matches full paths with case-insensitive subsequences", () => {
  const paths = ["src/store.js", "src/style.css", "README.md"];
  deepStrictEqual(top(paths, " SSJ "), ["src/store.js"]);
  strictEqual(fileSearch(paths).rank("zzz").length, 0);
  deepStrictEqual(fileSearch(paths).rank(""), { length: 3, ranked: null });
  deepStrictEqual(fileSearch([]).rank(""), { length: 0, ranked: null });
});

test("file search ranks by where the letters land", () => {
  // A run starting the file's name beats letters spread across folders.
  deepStrictEqual(top(["src/models/legal.ts", "src/components/Modal.vue"], "modal"), ["src/components/Modal.vue", "src/models/legal.ts"]);
  // Word starts and camelCase humps beat letters inside words.
  deepStrictEqual(top(["src/gitformat.ts", "web/src/components/GoToFileModal.vue"], "gtfm"), ["web/src/components/GoToFileModal.vue", "src/gitformat.ts"]);
  // A run beats the leftmost letters, even when those come first.
  deepStrictEqual(top(["src/s/t/o/r/e.js", "web/src/store.js"], "store"), ["web/src/store.js", "src/s/t/o/r/e.js"]);
  // Equal scores rank the shorter path, then the earlier listed.
  deepStrictEqual(top(["src/sub/store.js", "src/store.js", "lib/store.js"], "store"), ["src/store.js", "lib/store.js", "src/sub/store.js"]);
});

test("file search ranks files git ignores after the project's", () => {
  const paths = ["web/src/components/UnsavedModal.vue", "node_modules/ui/modal.ts"];
  deepStrictEqual(top(paths, "modal"), ["node_modules/ui/modal.ts", "web/src/components/UnsavedModal.vue"]);
  deepStrictEqual(top(paths, "modal", 5, 1), ["web/src/components/UnsavedModal.vue", "node_modules/ui/modal.ts"]);
});

test("file search answers the same whether typed, erased, retyped or asked afresh", () => {
  const paths = ["src/components/Button.vue", "src/store.js", "docs/setup.md", "Makefile", "app/main.go", "src/sub/store-test.js"];
  const typed = fileSearch(paths);
  for (const q of ["s", "st", "sto", "stor", "sto", "st", "sm", "s", "", "mk", "store", "x"]) {
    deepStrictEqual(typed.rank(q, true), fileSearch(paths).rank(q, true), q);
  }
});

test("file search ranks the best 100 first, and the rest when asked", () => {
  const paths = Array.from({ length: 300 }, (_, i) => `dir/${String(i).padStart(3, "0")}/file.txt`);
  const search = fileSearch(paths);
  const best = search.rank("file");
  strictEqual(best.length, 300);
  strictEqual(best.ranked.length, 100);
  const all = search.rank("file", true);
  strictEqual(all.ranked.length, 300);
  deepStrictEqual([...all.ranked.slice(0, 100)], [...best.ranked]);
  deepStrictEqual([...all.ranked], paths.map((_, i) => i));
});

test("match hits are the letters of the placement scored", () => {
  deepStrictEqual(matchHits("src/models/modal.ts", "modal"), [11, 12, 13, 14, 15]);
  deepStrictEqual(matchHits("web/src/components/GoToFileModal.vue", "GTFM"), [19, 21, 23, 27]);
  strictEqual(matchHits("src/store.js", "zz"), null);
});
