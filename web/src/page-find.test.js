import test from "node:test";
import assert from "node:assert/strict";
import { findMatches } from "./page-find.js";

test("find is literal, case insensitive, and non-overlapping", () => {
  assert.deepEqual(findMatches("A.b a.B axb", "a.b"), [{ start: 0, end: 3 }, { start: 4, end: 7 }]);
  assert.deepEqual(findMatches("aaaa", "aa"), [{ start: 0, end: 2 }, { start: 2, end: 4 }]);
  assert.deepEqual(findMatches("abc", ""), []);
  assert.deepEqual(findMatches("abc", "missing"), []);
  assert.deepEqual(findMatches("[x]", "[x]"), [{ start: 0, end: 3 }]);
});
test("Unicode matching keeps original DOM offsets", () => {
  assert.deepEqual(findMatches("İ 😀 TEST", "test"), [{ start: 5, end: 9 }]);
});
