import { strictEqual } from "node:assert/strict";
import { test } from "node:test";
import { fileInfoLabel, fileSize } from "./file-info.js";

test("file sizes use one decimal and promote rounded unit boundaries", () => {
  for (const [bytes, expected] of [
    [0, "0.0 B"], [12, "12.0 B"], [999, "999.0 B"],
    [1000, "1.0 KB"], [32400, "32.4 KB"], [12300000, "12.3 MB"],
    [999949, "999.9 KB"], [999950, "1.0 MB"], [1e9, "1.0 GB"],
    [1e12, "1.0 TB"], [null, ""], [undefined, ""], [-1, ""], [Infinity, ""],
  ]) strictEqual(fileSize(bytes), expected);
});

test("file information combines known size, dimensions and stored bit depth", () => {
  strictEqual(fileInfoLabel(null), "");
  strictEqual(fileInfoLabel({ size: 0 }), "(0.0 B)");
  strictEqual(fileInfoLabel({ size: 32200, image: { width: 320, height: 420, bits: 24 } }), "(32.2 KB, 320x420 24-bit)");
  strictEqual(fileInfoLabel({ size: 1000 }, { width: 32, height: 40 }), "(1.0 KB, 32x40)");
  strictEqual(fileInfoLabel({ size: 1000, image: { width: 0, height: 40, bits: 24 } }), "(1.0 KB)");
});
