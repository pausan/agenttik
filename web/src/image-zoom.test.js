import { ok, strictEqual } from "node:assert/strict";
import { test } from "node:test";
import { clampZoom, stepZoom, wheelZoom, ZOOMS, zoomKey } from "./image-zoom.js";

test("zoom steps stop at both ends and snap an odd scale to the next step", () => {
  strictEqual(stepZoom(1, 1), 1.25);
  strictEqual(stepZoom(1, -1), 0.75);
  strictEqual(stepZoom(ZOOMS.at(-1), 1), ZOOMS.at(-1));
  strictEqual(stepZoom(ZOOMS[0], -1), ZOOMS[0]);
  strictEqual(stepZoom(0.4, 1), 0.5);
  strictEqual(stepZoom(0.4, -1), 0.33);
  strictEqual(clampZoom(100), ZOOMS.at(-1));
  strictEqual(clampZoom(0), ZOOMS[0]);
});

test("zoom keys need Ctrl or Cmd and read + = - _ 0", () => {
  const key = (key, mods = { ctrlKey: true }) => zoomKey({ key, ...mods });
  strictEqual(key("+"), "in");
  strictEqual(key("="), "in");
  strictEqual(key("-"), "out");
  strictEqual(key("_"), "out");
  strictEqual(key("0"), "reset");
  strictEqual(key("+", { metaKey: true }), "in");
  strictEqual(key("+", {}), null);
  strictEqual(key("+", { ctrlKey: true, altKey: true }), null);
  strictEqual(key("a"), null);
});

test("a wheel down zooms out, up zooms in, and one event is capped", () => {
  ok(wheelZoom(100) < 1);
  ok(wheelZoom(-100) > 1);
  strictEqual(wheelZoom(1000), wheelZoom(50));
  strictEqual(wheelZoom(3, 1), wheelZoom(48));
  ok(Math.abs(wheelZoom(2) - 1) < 0.02);
});
