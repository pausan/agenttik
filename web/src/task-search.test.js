import test from "node:test";
import assert from "node:assert/strict";
import { dot, filterTaskRows, secondsLeft, taskText } from "./task-search.js";

const now = 1800000000000;
const tasks = [
  { id: "a", title: "Fix authentication", prompt: "database", last_active_at: now, done_at: 0 },
  { id: "b", title: "Database backup", last_active_at: now - 2 * 86400000, done_at: now },
  { id: "c", title: "Polish UI", last_active_at: now - 10 * 86400000 },
  { id: "d", title: "Old task", last_active_at: now - 31 * 86400000 },
];
const ids = (rows) => rows.map((r) => r.task.id);

test("fuzzy search matches titles only and preserves open/archive order", () => {
  assert.deepEqual(ids(filterTaskRows(tasks, "dbb", "all", null, now)), ["b"]);
  assert.deepEqual(ids(filterTaskRows(tasks, "", "all", null, now)), ["a", "b", "c", "d"]);
  assert.equal(filterTaskRows(tasks, "database", "all", null, now)[0].archived, true);
});
test("semantic search ranks across archive states and removes weak/missing scores", () => {
  assert.deepEqual(ids(filterTaskRows(tasks, "query", "all", { a: 0.4, b: 0.8, c: 0.29 }, now)), ["b", "a"]);
  assert.deepEqual(ids(filterTaskRows(tasks, "query", "all", {}, now)), []);
  assert.equal(filterTaskRows(tasks, "query", "all", { a: NaN }, now).length, 0);
});
test("time windows use last activity with exact inclusive boundaries", () => {
  for (const [window, expected] of [["1d", ["a"]], ["7d", ["a", "b"]], ["1mo", ["a", "b", "c"]], ["all", ["a", "b", "c", "d"]]]) {
    assert.deepEqual(ids(filterTaskRows(tasks, "", window, null, now)), expected);
  }
  assert.equal(filterTaskRows([{ id: "edge", last_active_at: now - 86400000 }], "", "1d", null, now).length, 1);
  assert.deepEqual(ids(filterTaskRows(tasks, "query", "1d", { a: 0.4, b: 0.9 }, now)), ["a"]);
});
test("blank smart queries preserve the ordinary list and task metadata supplies semantic text", () => {
  assert.deepEqual(ids(filterTaskRows(tasks, "  ", "all", {}, now)), ["a", "b", "c", "d"]);
  assert.equal(taskText({ prompt: "Fix login", summary: "Added tests" }), "Untitled task\nFix login\nAdded tests");
});
test("ETA and normalized dot product", () => {
  assert.equal(secondsLeft(0, 10, 500), null);
  assert.equal(secondsLeft(2, 10, 3000), 12);
  assert.equal(secondsLeft(10, 10, 3000), 0);
  assert.equal(dot([1, 0], [0, 1]), 0);
  assert.equal(dot([0.6, 0.8], [0.6, 0.8]), 1);
  assert.throws(() => dot([1], [1, 2]));
});
