import { test } from "node:test";
import { deepStrictEqual, strictEqual } from "node:assert";
import { searchFiles } from "./file-search.js";

test("file search matches full paths with case-insensitive subsequences", () => {
  const result = searchFiles(["src/store.js", "src/style.css", "README.md"], " SSJ ");
  deepStrictEqual(result.map((r) => r.path), ["src/store.js"]);
  deepStrictEqual(result[0].hits, [0, 4, 10]);
  deepStrictEqual(searchFiles(["src/store.js"], "zzz"), []);
});

test("file search ranks before limiting and bounds the unfiltered list", () => {
  deepStrictEqual(searchFiles(["long/path/a.txt", "a.txt"], "at", 1).map((r) => r.path), ["a.txt"]);
  strictEqual(searchFiles(Array.from({ length: 200 }, (_, i) => `${i}.txt`), "").length, 100);
  deepStrictEqual(searchFiles([], ""), []);
});
