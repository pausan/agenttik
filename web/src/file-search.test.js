import { test } from "node:test";
import { deepStrictEqual, strictEqual } from "node:assert";
import { fuzzy } from "./fuzzy.js";
import { searchFiles } from "./file-search.js";

const paths = (result) => Array.from({ length: result.length }, (_, i) => result.at(i).path);

test("file search matches full paths with case-insensitive subsequences", () => {
  const result = searchFiles(["src/store.js", "src/style.css", "README.md"], " SSJ ");
  deepStrictEqual(paths(result), ["src/store.js"]);
  deepStrictEqual(result.at(0).hits, [0, 4, 10]);
  strictEqual(searchFiles(["src/store.js"], "zzz").length, 0);
});

test("file search ranks by score, then length, then listing order", () => {
  deepStrictEqual(paths(searchFiles(["long/path/a.txt", "a.txt"], "at")), ["a.txt", "long/path/a.txt"]);
  deepStrictEqual(paths(searchFiles(["b/x", "a/x", "c/xy"], "x")), ["b/x", "a/x", "c/xy"]);
  const all = Array.from({ length: 200 }, (_, i) => `${i}.txt`);
  const result = searchFiles(all, "");
  strictEqual(result.length, 200);
  deepStrictEqual(result.at(5), { path: "5.txt", hits: null });
  strictEqual(searchFiles([], "").length, 0);
});

test("file search scores as fuzzy does, whether typed, erased or retyped", () => {
  const list = ["src/components/Button.vue", "src/store.js", "docs/setup.md", "Makefile", "app/main.go", "src/sub/store-test.js"];
  const expected = (q) => list
    .map((path, i) => ({ path, i, m: fuzzy(path, q) }))
    .filter((r) => r.m)
    .sort((a, b) => a.m.score - b.m.score || a.path.length - b.path.length || a.i - b.i)
    .map((r) => r.path);
  for (const q of ["s", "st", "sto", "stor", "sto", "st", "sm", "s", "", "mk", "store", "x"]) {
    deepStrictEqual(paths(searchFiles(list, q)), q ? expected(q) : list, q);
  }
});
