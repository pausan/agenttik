import test from "node:test";
import assert from "node:assert/strict";
import { durationLabel, taskSeconds, upsertBackgroundMessage } from "./background-tasks.js";

test("a task starts after current prose and updates without moving or splitting later prose", () => {
  const before = { role: "assistant", content: "Starting tests", streaming: true };
  const messages = [before];
  upsertBackgroundMessage(messages, { id: 2, role: "background", content: "running" });
  assert.equal(before.streaming, false);
  const after = { role: "assistant", content: "Checking files", streaming: true };
  messages.push({ role: "tool", content: "Read files" }, after);
  upsertBackgroundMessage(messages, { id: 2, role: "background", content: "completed" });
  assert.deepEqual(messages.map((m) => m.content), ["Starting tests", "completed", "Read files", "Checking files"]);
  assert.equal(after.streaming, true);
});

test("repeated events after a reload update the saved row without duplicating it", () => {
  const messages = [{ id: 5, role: "background", content: "running" }, { id: 6, role: "tool" }];
  const update = { id: 5, role: "background", content: "failed" };
  upsertBackgroundMessage(messages, update);
  upsertBackgroundMessage(messages, update);
  assert.equal(messages.length, 2);
  assert.equal(messages[0].content, "failed");
});

test("tasks with the same provider id in different turns have separate message ids", () => {
  const messages = [];
  upsertBackgroundMessage(messages, { id: 1, turn_id: 1, content: '{"id":"b1","status":"completed"}' });
  upsertBackgroundMessage(messages, { id: 2, turn_id: 2, content: '{"id":"b1","status":"running"}' });
  assert.equal(messages.length, 2);
  assert.equal(messages[0].turn_id, 1);
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
