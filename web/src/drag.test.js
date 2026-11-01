import assert from "node:assert/strict";
import test from "node:test";
import { beginDrag } from "./drag.js";

function setup(t, count = 4, role) {
  const original = globalThis.document;
  const document = new EventTarget();
  globalThis.document = document;
  t.after(() => {
    document.dispatchEvent(new Event("dragend"));
    globalThis.document = original;
  });
  const parent = { children: [] };
  const row = (index, group) => {
    const at = 20 + parent.children.length * 40;
    const element = {
      dataset: { ...(index === null ? {} : { dragIndex: String(index) }), ...(group ? { dragGroup: group } : {}) },
      parentElement: parent,
      getAttribute() { return role; },
      getBoundingClientRect() {
        return role === "tab"
          ? { left: at, top: 0, width: 40, height: 20 }
          : { left: 0, top: at, width: 100, height: 40 };
      },
      removeAttribute() { delete this.dataset.dropPosition; },
      closest() { return index === null ? null : this; },
    };
    parent.children.push(element);
    return element;
  };
  const rows = Array.from({ length: count }, (_, i) => row(i));
  const previews = [];
  const start = (from, drop = () => {}, preview = from) => beginDrag({
    currentTarget: from, clientX: 5, clientY: from.getBoundingClientRect().top + 5,
    dataTransfer: { setData() {}, setDragImage(...args) { previews.push(args); } },
  }, drop, preview);
  const at = (position) => role === "tab" ? { clientX: position, clientY: 500 } : { clientX: 900, clientY: position };
  const fire = (type, position) => {
    const event = Object.assign(new Event(type, { cancelable: true }), at(position));
    document.dispatchEvent(event);
    return event;
  };
  const marked = () => parent.children.flatMap((candidate, i) =>
    candidate.dataset.dropPosition ? [[i, candidate.dataset.dropPosition]] : []);
  return { document, row, rows, start, fire, marked, previews };
}

test("the line follows the nearest edge along the list", (t) => {
  const { rows, start, fire, marked } = setup(t);
  start(rows[0]);
  fire("dragover", 110); // the upper half of the third row
  assert.deepEqual(marked(), [[2, "before"]]);
  fire("dragover", 90); // the lower half of the second row
  assert.deepEqual(marked(), [[2, "before"]]);
  fire("dragover", 70); // the upper half of the second row
  assert.deepEqual(marked(), [[1, "before"]]);
  fire("dragover", 170); // the lower half of the last row
  assert.deepEqual(marked(), [[3, "after"]]);
});

test("the line stays on a row wherever the pointer goes", (t) => {
  const { rows, start, fire, marked } = setup(t);
  start(rows[1]);
  for (const [position, edge] of [[-500, [0, "before"]], [5000, [3, "after"]]]) {
    assert.equal(fire("dragover", position).defaultPrevented, true);
    assert.deepEqual(marked(), [edge]);
  }
});

test("over its own row the line sits on the near edge and a drop changes nothing", (t) => {
  const { rows, start, fire, marked } = setup(t);
  const drops = [];
  start(rows[1], (...args) => drops.push(args));
  fire("dragover", 65);
  assert.deepEqual(marked(), [[1, "before"]]);
  fire("dragover", 75);
  assert.deepEqual(marked(), [[1, "before"]]);
  fire("dragover", 85);
  assert.deepEqual(marked(), [[2, "before"]]);
  fire("drop", 85);
  assert.deepEqual(drops, []);
  assert.deepEqual(marked(), []);
});

test("a drop lands on the line in either direction", (t) => {
  const { rows, start, fire } = setup(t);
  const drops = [];
  const drop = (...args) => drops.push(args);
  start(rows[0], drop);
  fire("drop", 95);
  start(rows[0], drop);
  fire("drop", 5000);
  start(rows[3], drop);
  fire("drop", -500);
  assert.deepEqual(drops, [
    [0, 1, 2, false],
    [0, 3, 3, true],
    [3, 0, 0, false],
  ]);
});

test("rows without an index or in another group are never landed on", (t) => {
  const { row, rows, start, fire, marked } = setup(t, 2);
  const pinned = row(null);
  const other = row(2, "file");
  const drops = [];
  start(rows[0], (...args) => drops.push(args));
  fire("dragover", 110); // over the pinned row
  assert.deepEqual(marked(), [[1, "after"]]);
  fire("dragover", 150); // over the other group
  assert.deepEqual(marked(), [[1, "after"]]);
  assert.equal(pinned.dataset.dropPosition, undefined);
  assert.equal(other.dataset.dropPosition, undefined);
  fire("drop", 150);
  assert.deepEqual(drops, [[0, 1, 1, true]]);
});

test("tabs choose by their left and right halves", (t) => {
  const { rows, start, fire, marked } = setup(t, 3, "tab");
  start(rows[0]);
  fire("dragover", 95);
  assert.deepEqual(marked(), [[2, "before"]]);
  fire("dragover", 5000);
  assert.deepEqual(marked(), [[2, "after"]]);
});

test("a source removed during the drag cancels it", (t) => {
  const { rows, start, fire, marked } = setup(t);
  const drops = [];
  start(rows[1], (...args) => drops.push(args));
  fire("dragover", 25);
  rows[1].parentElement = null;
  fire("dragover", 25);
  assert.deepEqual(marked(), []);
  fire("drop", 25);
  assert.deepEqual(drops, []);
});

test("the preview can be a whole row", (t) => {
  const { rows, start, previews } = setup(t);
  start(rows[1], undefined, rows[0]);
  assert.deepEqual(previews[0], [rows[0], 5, 45]);
});

test("a row outside any list leaves the drag to the browser", (t) => {
  const { row, start, fire } = setup(t);
  start(row(null));
  assert.equal(fire("dragover", 25).defaultPrevented, false);
});

test("cancellation and new drags clear the marker", (t) => {
  const { document, rows, start, fire, marked } = setup(t);
  const drops = [];
  start(rows[0], (...args) => drops.push(args));
  fire("dragover", 155);
  start(rows[1]);
  assert.deepEqual(marked(), []);
  fire("dragover", 155);
  document.dispatchEvent(new Event("dragend"));
  assert.deepEqual(marked(), []);
  assert.equal(fire("dragover", 155).defaultPrevented, false);
  assert.deepEqual(drops, []);
});
