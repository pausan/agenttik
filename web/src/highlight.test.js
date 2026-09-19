import test from "node:test";
import assert from "node:assert/strict";
import { highlight, highlightLines } from "./highlight.js";

test("line highlighting carries block state and preserves escaped source", () => {
  const lines = highlightLines('/* start\n<a>&b\n*/ const value = "<script>";', "js");
  assert.match(lines[1], /hl-c/);
  assert.match(lines[1], /&lt;a&gt;&amp;b/);
  assert.match(lines[2], /hl-k/);
  assert.ok(!lines.join("\n").includes("<script>"));
  const edited = highlightLines('// start\n<a>&b\n*/ const value = "<script>";', "js");
  assert.doesNotMatch(edited[1], /hl-c/);
});

test("line highlighting preserves empty lines and trailing newlines", () => {
  assert.deepEqual(highlightLines("a\n\n", ""), ["a", "", ""]);
  assert.deepEqual(highlightLines("", ""), [""]);
  assert.equal(highlight("<a>\n&", ""), "&lt;a&gt;\n&amp;");
});
