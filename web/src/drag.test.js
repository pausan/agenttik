import assert from "node:assert/strict";
import test from "node:test";
import { beginDrag, showDropPosition } from "./drag.js";

function setup(t) {
  const original = globalThis.document;
  const document = new EventTarget();
  document.createElement = () => ({ style: {}, setAttribute() {} });
  document.body = { append() {} };
  globalThis.document = document;
  t.after(() => {
    document.dispatchEvent(new Event("dragend"));
    globalThis.document = original;
  });
  const row = () => ({
    dataset: {},
    removeAttribute() { delete this.dataset.dropPosition; },
  });
  const start = (source, marker = source) => beginDrag({
    currentTarget: source,
    dataTransfer: { setData() {}, setDragImage() {} },
  }, "item", marker);
  return { document, row, start };
}

test("the insertion edge follows movement and survives hovering the moved row", (t) => {
  const { row, start } = setup(t);
  const source = row();
  start(source);
  showDropPosition(0, 2);
  assert.equal(source.dataset.dropPosition, "after");
  showDropPosition(2, 2);
  assert.equal(source.dataset.dropPosition, "after");
  showDropPosition(2, 0);
  assert.equal(source.dataset.dropPosition, "before");
});

test("invalid targets, drops, cancellation and new drags clear the marker", (t) => {
  const { document, row, start } = setup(t);
  const source = row(), project = row(), next = row();
  start(source, project);
  showDropPosition(0, 1);
  assert.equal(source.dataset.dropPosition, undefined);
  assert.equal(project.dataset.dropPosition, "after");
  const accepted = new Event("dragover", { cancelable: true });
  accepted.preventDefault();
  document.dispatchEvent(accepted);
  assert.equal(project.dataset.dropPosition, "after");
  document.dispatchEvent(new Event("dragover"));
  assert.equal(project.dataset.dropPosition, undefined);
  showDropPosition(1, 1);
  assert.equal(project.dataset.dropPosition, "after");
  start(next);
  assert.equal(project.dataset.dropPosition, undefined);
  showDropPosition(0, 0);
  assert.equal(next.dataset.dropPosition, "before");
  document.dispatchEvent(new Event("drop"));
  assert.equal(next.dataset.dropPosition, undefined);
  showDropPosition(0, 1);
  assert.equal(next.dataset.dropPosition, undefined);
  start(next);
  showDropPosition(0, 1);
  document.dispatchEvent(new Event("dragend"));
  assert.equal(next.dataset.dropPosition, undefined);
});
