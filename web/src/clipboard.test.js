import test from "node:test";
import assert from "node:assert/strict";
import { writeClipboard } from "./clipboard.js";

/* A document whose copy command runs the listeners the way a browser does. */
function fakeDocument(works = true) {
  const listeners = new Map();
  const doc = {
    written: "",
    addEventListener: (type, fn) => listeners.set(type, fn),
    removeEventListener: (type) => listeners.delete(type),
    execCommand(command) {
      assert.equal(command, "copy");
      if (!works) return false;
      let cancelled = 0;
      const event = { preventDefault: () => cancelled++, clipboardData: { setData: (type, value) => { assert.equal(type, "text/plain"); doc.written = value; } } };
      listeners.get("beforecopy")(event);
      listeners.get("copy")(event);
      assert.equal(cancelled, 2);
      return true;
    },
    listeners,
  };
  return doc;
}

test("the clipboard API is used when it works", async () => {
  const calls = [];
  const doc = fakeDocument();
  await writeClipboard("text", { writeText: async (text) => calls.push(text) }, doc);
  assert.deepEqual(calls, ["text"]);
  assert.equal(doc.written, "");
});

test("plain HTTP has no clipboard API and uses the copy command", async () => {
  const doc = fakeDocument();
  await writeClipboard("https://example.com", undefined, doc);
  assert.equal(doc.written, "https://example.com");
  assert.equal(doc.listeners.size, 0);
});

test("a refused clipboard API uses the copy command", async () => {
  const doc = fakeDocument();
  await writeClipboard("text", { writeText: async () => { throw new DOMException("Write permission denied.", "NotAllowedError"); } }, doc);
  assert.equal(doc.written, "text");
});

test("a refused copy command reports the failure and leaves no listeners", async () => {
  const doc = fakeDocument(false);
  await assert.rejects(writeClipboard("text", undefined, doc), /Could not copy to the clipboard/);
  assert.equal(doc.listeners.size, 0);
});
