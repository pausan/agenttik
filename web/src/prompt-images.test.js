import { test } from "node:test";
import assert from "node:assert/strict";
import { clipboardImages, promptImages, promptText, withImages } from "./prompt-images.js";

const image = { reference: `![Attached image](/api/attachments/${"a".repeat(64)}.png)`, url: `/api/attachments/${"a".repeat(64)}.png` };

test("image drafts round trip without changing typed whitespace", () => {
  for (const text of ["", "hello", "hello\n\n", "\n"]) {
    const draft = withImages(text, [image, image]);
    assert.equal(promptText(draft), text);
    assert.deepEqual(promptImages(draft), [image, image]);
    assert.equal(promptText(text), text);
  }
});

test("clipboard collects every image and leaves text paste alone", () => {
  const file = { name: "shot.png" };
  const item = { kind: "file", type: "image/png", getAsFile: () => file };
  assert.deepEqual(clipboardImages({ items: [item, { kind: "string", type: "text/plain" }, item] }), [file, file]);
  assert.deepEqual(clipboardImages(null), []);
});
