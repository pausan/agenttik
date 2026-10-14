import { test } from "node:test";
import assert from "node:assert/strict";
import { areaQuote, imageArea, imagePoint, markedImage } from "./image-area.js";

test("image coordinates ignore display scale and clamp drags to the image", () => {
  assert.deepEqual(imagePoint(150, 100, { left: 50, top: 50, width: 200, height: 100 }), { x: 0.5, y: 0.5 });
  assert.deepEqual(imagePoint(250, 150, { left: 50, top: 50, width: 400, height: 200 }), { x: 0.5, y: 0.5 });
  assert.deepEqual(imagePoint(-20, 300, { left: 50, top: 50, width: 200, height: 100 }), { x: 0, y: 1 });
});

test("drag direction does not change the selected area or its original pixel description", () => {
  const start = { x: 0.25, y: 0.25 }, end = { x: 0.75, y: 0.75 };
  const area = { x: 0.25, y: 0.25, width: 0.5, height: 0.5 };
  assert.deepEqual(imageArea(start, end), area);
  assert.deepEqual(imageArea(end, start), area);
  assert.equal(areaQuote("chart.png", area, { naturalWidth: 4000, naturalHeight: 2000 }),
    "Circled area of chart.png: x 1000–3000, y 500–1500 in the 4000 × 2000 image. The attached image marks this area in red.");
});

test("marked copies bound canvas memory and keep the circle at the same image coordinates", async (t) => {
  const calls = [];
  const ctx = {
    drawImage: (...args) => calls.push(args), beginPath() {},
    ellipse: (...args) => calls.push(args), stroke() {},
  };
  const canvas = { getContext: () => ctx, toBlob: (done, type) => done(new Blob(["image"], { type })) };
  const previous = globalThis.document;
  globalThis.document = { createElement: () => canvas };
  t.after(() => {
    if (previous === undefined) delete globalThis.document;
    else globalThis.document = previous;
  });
  const image = { naturalWidth: 8192, naturalHeight: 4096 };
  const blob = await markedImage(image, { x: 0.25, y: 0.25, width: 0.5, height: 0.5 });
  assert.equal(blob.type, "image/png");
  assert.equal(canvas.width, 2048);
  assert.equal(canvas.height, 1024);
  assert.deepEqual(calls[0], [image, 0, 0, 2048, 1024]);
  assert.deepEqual(calls[1], [1024, 512, 512, 256, 0, 0, Math.PI * 2]);
  canvas.toBlob = (done, type) => done(new Blob([new Uint8Array(type === "image/png" ? 4 * 1024 * 1024 + 1 : 1)], { type }));
  assert.equal((await markedImage(image, { x: 0, y: 0, width: 1, height: 1 })).type, "image/jpeg");
});
