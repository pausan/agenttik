import { test } from "node:test";
import assert from "node:assert/strict";
import { clipboardImages, promptImages, promptText, readClipboardImages, uploadImage, withImages } from "./prompt-images.js";

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

test("file-list-only clipboard images are accepted without duplicates", () => {
  const file = { type: "image/png" };
  const item = { kind: "file", type: file.type, getAsFile: () => file };
  assert.deepEqual(clipboardImages({ files: [file] }), [file]);
  assert.deepEqual(clipboardImages({ items: [item], files: [file] }), [file]);
});

test("clipboard API reads one image format per item and preserves multiple images", async () => {
  const images = [new Blob(["one"], { type: "image/png" }), new Blob(["two"], { type: "image/png" })];
  const clipboard = { read: async () => [
    ...images.map((image) => ({ types: ["text/html", "image/png", "image/jpeg"], getType: async (type) => {
      assert.equal(type, "image/png");
      return image;
    } })),
    { types: ["text/plain"], getType: () => { throw new Error("should not read text"); } },
  ] };
  assert.deepEqual(await readClipboardImages(clipboard), images);
});

test("image uploads send byte buffers rather than native WebKit Blob bodies", async (t) => {
  const bytes = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0, 0xff]);
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "/api/attachments");
    assert.equal(options.method, "POST");
    assert.ok(options.body instanceof ArrayBuffer);
    assert.deepEqual(new Uint8Array(options.body), bytes);
    assert.equal(options.headers["Content-Type"], "image/png");
    return new Response(JSON.stringify({ url: image.url }));
  });
  for (const file of [new Blob([bytes], { type: "image/png" }), new File([bytes], "pasted.png", { type: "image/png" })]) {
    assert.deepEqual(await uploadImage(file), image);
  }
});

test("oversized images are rejected before allocating upload bytes", async (t) => {
  t.mock.method(globalThis, "fetch", () => assert.fail("must not upload"));
  await assert.rejects(uploadImage({
    size: 4 * 1024 * 1024 + 1,
    arrayBuffer: () => assert.fail("must not read oversized image"),
  }), /4 MiB or smaller/);
});
