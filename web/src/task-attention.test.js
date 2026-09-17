import test from "node:test";
import assert from "node:assert/strict";
import { reactive, ref } from "vue";
import { trackTaskAttention, taskDot } from "./task-attention.js";

test("reading requires three uninterrupted visible seconds and restarts for a new completion", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const unread = reactive({ a: 1, b: 2 });
  const selected = ref("a");
  const visible = ref(true);
  let saves = 0;
  const stop = trackTaskAttention(unread, () => selected.value, () => visible.value, () => saves++);
  t.after(stop);
  t.mock.timers.tick(2999);
  assert.equal(unread.a, 1);
  selected.value = "b";
  t.mock.timers.tick(1000);
  selected.value = "a";
  t.mock.timers.tick(2000);
  visible.value = false;
  t.mock.timers.tick(4000);
  assert.equal(unread.a, 1);
  visible.value = true;
  t.mock.timers.tick(2000);
  unread.a = 3;
  t.mock.timers.tick(2999);
  assert.equal(unread.a, 3);
  t.mock.timers.tick(1);
  assert.equal(unread.a, undefined);
  assert.equal(unread.b, 2);
  assert.equal(saves, 1);
});

test("completion while viewing starts its own timer; leaving the transcript cancels it", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const unread = reactive({});
  const selected = ref("a");
  const stop = trackTaskAttention(unread, () => selected.value, () => true);
  t.after(stop);
  t.mock.timers.tick(10000);
  unread.a = 1;
  t.mock.timers.tick(2000);
  selected.value = null;
  t.mock.timers.tick(4000);
  assert.equal(unread.a, 1);
  selected.value = "a";
  t.mock.timers.tick(3000);
  assert.equal(unread.a, undefined);
});

test("error red survives acknowledgement and running keeps its activity dot", () => {
  assert.equal(taskDot("idle", true), "unread");
  assert.equal(taskDot("idle", false), "idle");
  assert.equal(taskDot("error", true), "error");
  assert.equal(taskDot("error", false), "error");
  assert.equal(taskDot("running", true), "running");
  assert.equal(taskDot("waiting", false), "waiting");
});
