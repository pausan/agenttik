import { test } from "node:test";
import assert from "node:assert/strict";
import { effectScope, shallowRef } from "vue";
import { useCommitFiles } from "./commitFiles.js";

function setup(t, count) {
  const source = shallowRef(makeFiles(count));
  const scope = effectScope();
  t.after(() => scope.stop());
  return { source, ...scope.run(() => useCommitFiles(() => source.value)) };
}

const makeFiles = (count) => Array.from({ length: count }, (_, i) => ({ path: `file-${i}` }));

test("up to 1000 files are shown in full, including empty and loading lists", (t) => {
  const { source, visible, more } = setup(t, 0);
  for (const count of [0, 250, 999, 1000]) {
    source.value = makeFiles(count);
    assert.equal(visible.value.length, count);
    assert.equal(more.value, 0);
  }
  source.value = null;
  assert.deepEqual(visible.value, []);
  assert.equal(more.value, 0);
});

test("large commits reveal ordered batches of 250, 250, 500, then 1000 each", (t) => {
  const { source, visible, more, showMore } = setup(t, 4250);
  for (const [count, next] of [[250, 250], [500, 500], [1000, 1000], [2000, 1000], [3000, 1000], [4000, 250], [4250, 0]]) {
    assert.deepEqual(visible.value, source.value.slice(0, count));
    assert.equal(more.value, next);
    showMore();
  }
  assert.equal(visible.value.length, 4250);
});

test("1001 files use batches and offer only the remaining file at the end", (t) => {
  const { visible, more, showMore } = setup(t, 1001);
  assert.equal(visible.value.length, 250);
  showMore();
  showMore();
  assert.equal(visible.value.length, 1000);
  assert.equal(more.value, 1);
  showMore();
  assert.equal(visible.value.length, 1001);
  assert.equal(more.value, 0);
});

test("switching commits and reopening cached files reset the visible batch", (t) => {
  const { source, visible, more, showMore } = setup(t, 3000);
  const cached = source.value;
  showMore();
  showMore();
  source.value = makeFiles(2000);
  assert.equal(visible.value.length, 250);
  assert.equal(more.value, 250);
  showMore();
  source.value = null;
  assert.equal(visible.value.length, 0);
  source.value = cached;
  assert.equal(visible.value.length, 250);
  assert.equal(more.value, 250);
});
