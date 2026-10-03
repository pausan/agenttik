import test from "node:test";
import assert from "node:assert/strict";
import { durationLabel, taskSeconds, tasksLabel, upsertTask } from "./background-tasks.js";

test("the latest copy of a task replaces the one before, oldest first", () => {
  let list = upsertTask(undefined, { id: "b", started_at: 20, status: "running" });
  list = upsertTask(list, { id: "a", started_at: 10, status: "running" });
  list = upsertTask(list, { id: "b", started_at: 20, status: "completed", ended_at: 30 });
  assert.deepEqual(list.map((t) => `${t.id}:${t.status}`), ["a:running", "b:completed"]);
});

test("a running task counts to now, a finished one to when it ended", () => {
  assert.equal(taskSeconds({ started_at: 1000 }, 43_999), 42);
  assert.equal(taskSeconds({ started_at: 1000, ended_at: 5000 }, 99_000), 4);
  assert.equal(taskSeconds({ started_at: 5000 }, 1000), 0);
});

test("durations read like a stopwatch past a minute", () => {
  assert.equal(durationLabel(0), "0s");
  assert.equal(durationLabel(59), "59s");
  assert.equal(durationLabel(65), "1:05");
  assert.equal(durationLabel(3723), "1:02:03");
});

test("the label counts running tasks, or finished ones when none runs", () => {
  assert.equal(tasksLabel([{ status: "running" }]), "1 background task running");
  assert.equal(tasksLabel([{ status: "running" }, { status: "running" }, { status: "completed" }]), "2 background tasks running");
  assert.equal(tasksLabel([{ status: "completed" }, { status: "failed" }]), "2 background tasks finished");
});
