import assert from "node:assert/strict";
import test from "node:test";
import { beginDrag, dropAfter, showDropPosition } from "./drag.js";

function setup(t) {
  const original = globalThis.document;
  const document = new EventTarget();
  globalThis.document = document;
  t.after(() => {
    document.dispatchEvent(new Event("dragend"));
    globalThis.document = original;
  });
  const row = (role) => ({
    dataset: {},
    getAttribute() { return role; },
    getBoundingClientRect() { return { left: 10, top: 20, width: 100, height: 40 }; },
    contains(target) { return target === this; },
    removeAttribute() { delete this.dataset.dropPosition; },
  });
  const hover = (target, after = false) => ({
    currentTarget: target, clientX: after ? 90 : 30, clientY: after ? 55 : 25,
  });
  const previews = [];
  const start = (source, preview = source) => beginDrag({
    ...hover(source),
    dataTransfer: { setData() {}, setDragImage(...args) { previews.push(args); } },
  }, "item", preview);
  return { document, row, start, hover, previews };
}

test("both halves of rows and tabs map to the exact insertion index", (t) => {
  const { row, start, hover } = setup(t);
  for (const role of [undefined, "tab"]) {
    const target = row(role);
    start(row(role));
    assert.equal(dropAfter(hover(target)), false);
    assert.equal(dropAfter(hover(target, true)), true);
    assert.equal(showDropPosition(hover(target), 0, 2), 1);
    assert.equal(target.dataset.dropPosition, "before");
    assert.equal(showDropPosition(hover(target, true), 0, 2), 2);
    assert.equal(target.dataset.dropPosition, "after");
    assert.equal(showDropPosition(hover(target), 2, 0), 0);
    assert.equal(showDropPosition(hover(target, true), 2, 0), 1);
    assert.equal(showDropPosition(hover(target), 1, 2), 1);
    assert.equal(target.dataset.dropPosition, undefined);
    assert.equal(showDropPosition(hover(target, true), 1, 0), 1);
    assert.equal(target.dataset.dropPosition, undefined);
    assert.equal(showDropPosition(hover(target), 1, 1), 1);
    assert.equal(showDropPosition(hover(target, true), 1, 1), 1);
  }
});

test("the preview uses the source while the marker switches between targets", (t) => {
  const { row, start, hover, previews } = setup(t);
  const grip = row(), source = row(), first = row(), second = row();
  start(grip, source);
  assert.deepEqual(previews[0], [source, 20, 5]);
  showDropPosition(hover(first), 2, 0);
  assert.equal(first.dataset.dropPosition, "before");
  assert.equal(source.dataset.dropPosition, undefined);
  showDropPosition(hover(second, true), 2, 0);
  assert.equal(first.dataset.dropPosition, undefined);
  assert.equal(second.dataset.dropPosition, "after");
});

test("invalid targets, drops, cancellation and new drags clear the marker", (t) => {
  const { document, row, start, hover } = setup(t);
  const source = row(), target = row();
  start(source);
  showDropPosition(hover(target, true), 0, 2);
  const accepted = new Event("dragover", { cancelable: true });
  accepted.preventDefault();
  document.dispatchEvent(accepted);
  assert.equal(target.dataset.dropPosition, "after");
  document.dispatchEvent(new Event("dragover"));
  assert.equal(target.dataset.dropPosition, undefined);
  showDropPosition(hover(target), 2, 0);
  start(source);
  assert.equal(target.dataset.dropPosition, undefined);
  showDropPosition(hover(target), 2, 0);
  document.dispatchEvent(new Event("drop"));
  assert.equal(target.dataset.dropPosition, undefined);
  // Drop handlers can calculate the index after capture cleaned the marker.
  assert.equal(showDropPosition(hover(target, true), 2, 0), 1);
  assert.equal(target.dataset.dropPosition, undefined);
  start(source);
  showDropPosition(hover(target), 2, 0);
  document.dispatchEvent(new Event("dragend"));
  assert.equal(target.dataset.dropPosition, undefined);
});
