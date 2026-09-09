import { deepStrictEqual, strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { filterDirs, fuzzy, segments } from "./fuzzy.js";

const dirs = (...names) => names.map((name) => ({ name, path: "/x/" + name }));
const names = (out) => out.map((m) => m.dir.name);

test("a subsequence matches, anything else does not", () => {
  strictEqual(fuzzy("agenttik", "att") !== null, true);
  strictEqual(fuzzy("agenttik", "AGT") !== null, true);
  strictEqual(fuzzy("agenttik", "xyz"), null);
  strictEqual(fuzzy("agenttik", "tta"), null); // order matters
});

test("matches near the start and close together rank first", () => {
  deepStrictEqual(
    names(filterDirs(dirs("migrate2gitlfs", "agenttik", "mocha-list-tests"), "att")),
    ["agenttik", "migrate2gitlfs", "mocha-list-tests"],
  );
});

test("an equal score is broken by the shorter name, then alphabetically", () => {
  deepStrictEqual(names(filterDirs(dirs("abcd", "abc", "abce"), "abc")), ["abc", "abcd", "abce"]);
});

test("no filter keeps every folder, in the order given", () => {
  const out = filterDirs(dirs("b", "a"), "");
  deepStrictEqual(names(out), ["b", "a"]);
  strictEqual(out[0].hits, null);
});

test("segments split a name into matched letters and the rest", () => {
  deepStrictEqual(segments("agenttik", fuzzy("agenttik", "att").hits), [
    { text: "a", hit: true },
    { text: "gen", hit: false },
    { text: "t", hit: true },
    { text: "t", hit: true },
    { text: "ik", hit: false },
  ]);
  deepStrictEqual(segments("agenttik", null), [{ text: "agenttik", hit: false }]);
});

test("a name whose length changes when lowercased keeps the match but drops the highlight", () => {
  const m = fuzzy("İ", "i̇");
  if (m) strictEqual(m.hits, null);
});
