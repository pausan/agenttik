import test from "node:test";
import assert from "node:assert/strict";
import { largeFile, lineCount } from "./file-editor.js";

test("large editor thresholds include short files with many lines", () => {
  assert.equal(lineCount(""), 1);
  assert.equal(lineCount("a\n\n"), 3);
  assert.equal(largeFile("a\n".repeat(9998)), false);
  assert.equal(largeFile("a\n".repeat(9999)), true);
  assert.equal(largeFile("a".repeat(400_001)), true);
});
