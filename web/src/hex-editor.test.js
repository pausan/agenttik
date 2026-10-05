import test from "node:test";
import assert from "node:assert/strict";
import { HEX_PAGE_BYTES, hexRows, parseHex, replaceHex } from "./hex-editor.js";

test("hex input accepts byte groups and rejects incomplete or invalid values", () => {
  assert.equal(parseHex("00 FF\n80"), "00ff80");
  for (const value of ["", "f", "0xFF", "gg", "ff1"]) assert.equal(parseHex(value), null);
});

test("byte replacement preserves all bytes outside the selected range", () => {
  assert.equal(replaceHex("0041ff80", 1, "4243"), "00424380");
  assert.equal(replaceHex("0041ff80", 3, "fe"), "0041fffe");
  for (const offset of [-1, 4, 1.5]) assert.equal(replaceHex("0041ff80", offset, "01"), null);
  assert.equal(replaceHex("0041ff80", 3, "0102"), null);
});

test("hex rows bound decoding and preserve offsets, partial rows and ASCII", () => {
  assert.deepEqual(hexRows("0020417eff80", 0), [{ offset: 0, bytes: ["00", "20", "41", "7e", "ff", "80"], ascii: ". A~.." }]);
  const hex = "41".repeat(HEX_PAGE_BYTES + 3);
  assert.equal(hexRows(hex, 0).length, 32);
  assert.deepEqual(hexRows(hex, HEX_PAGE_BYTES), [{ offset: HEX_PAGE_BYTES, bytes: ["41", "41", "41"], ascii: "AAA" }]);
  assert.deepEqual(hexRows("", 0), []);
});
