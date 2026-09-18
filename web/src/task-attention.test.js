import test from "node:test";
import assert from "node:assert/strict";
import { reactive, ref } from "vue";
import { trackTaskAttention, taskDot, projectDot } from "./task-attention.js";

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

test("manually unread stays unread for the current visit, then clears after returning", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const unread = reactive({ a: 1 });
  const selected = ref("a");
  const visible = ref(true);
  const stop = trackTaskAttention(unread, () => selected.value, () => visible.value);
  t.after(stop);
  t.mock.timers.tick(3000);
  unread.a = -1;
  t.mock.timers.tick(4000);
  visible.value = false;
  visible.value = true;
  t.mock.timers.tick(4000);
  assert.equal(unread.a, -1);
  selected.value = null;
  selected.value = "a";
  t.mock.timers.tick(2999);
  assert.equal(unread.a, -1);
  t.mock.timers.tick(1);
  assert.equal(unread.a, undefined);
});

test("a new completion replaces a manual unread hold", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const unread = reactive({});
  const stop = trackTaskAttention(unread, () => "a", () => true);
  t.after(stop);
  unread.a = -1;
  unread.a = 7;
  t.mock.timers.tick(3000);
  assert.equal(unread.a, undefined);
});

test("project dots combine unread task attention with task and scheduled job activity", () => {
  const project = { recent_sessions: [{ id: "a", status: "idle" }, { id: "b", status: "idle" }], schedules: [] };
  const unread = { a: 1 };
  assert.equal(projectDot(project, {}), null);
  assert.equal(projectDot(project, unread), "unread");
  assert.equal(projectDot(project, { elsewhere: 1 }), null);
  project.recent_sessions[1].status = "running";
  assert.equal(projectDot(project, unread), "unread-running");
  assert.equal(projectDot(project, {}), "running");
  project.recent_sessions[1].status = "idle";
  project.schedules.push({ running: true });
  assert.equal(projectDot(project, unread), "unread-running");
  assert.equal(projectDot(project, {}), "running");
  project.schedules[0].running = false;
  assert.equal(projectDot(project, unread), "unread");
  project.recent_sessions[0].status = "error";
  assert.equal(projectDot(project, unread), null);
  project.recent_sessions[0].status = "running";
  assert.equal(projectDot(project, unread), "running");
  project.recent_sessions[0].status = "idle";
  assert.equal(projectDot(project, { a: -1 }), "unread");
  assert.equal(projectDot({ recent_sessions: [], schedules: [] }, unread), null);
});
