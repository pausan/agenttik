import test from "node:test";
import assert from "node:assert/strict";
import { durationLabel, promptState, runningTaskCount, takeFollowUp, taskSeconds, upsertBackgroundMessage, waitingLabel, wakesAgent } from "./background-tasks.js";

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

test("only running tasks count, and unreadable rows are skipped", () => {
  const messages = [
    { role: "background", content: '{"id":"a","status":"running"}' },
    { role: "background", content: '{"id":"b","status":"completed"}' },
    { role: "background", content: "not json" },
    { role: "assistant", content: '{"status":"running"}' },
  ];
  assert.equal(runningTaskCount(messages), 1);
});

test("a follow-up moves from its send-time bubble to after the reply it waited behind", () => {
  const reply = { role: "assistant", content: "Game A built", streaming: true };
  const messages = [
    { role: "user", content: "status?" },
    { id: 1, role: "assistant", content: "Builds started" },
    { role: "user", content: "status?" },
    reply,
  ];
  const stored = { id: 9, role: "user", content: "status?" };
  takeFollowUp(messages, stored);
  takeFollowUp(messages, stored);
  assert.deepEqual(messages.map((m) => m.id ?? m.content), ["status?", 1, "Game A built", 9]);
  assert.equal(reply.streaming, false);
});

test("a background subagent's tool calls do not wake the main agent", () => {
  assert.equal(wakesAgent({ type: "tool_use", tool: { name: "Bash", subagent: true } }), false);
  assert.equal(wakesAgent({ type: "tool_use", tool: { name: "Bash" } }), true);
  assert.equal(wakesAgent({ type: "text", text: "Game A built" }), true);
  assert.equal(wakesAgent({ type: "follow_up" }), true);
  assert.equal(wakesAgent({ type: "background" }), false);
});

test("a turn that takes prompts names its background tasks, working or waiting", () => {
  assert.equal(waitingLabel(true, 1), "Waiting for 1 background task · you can keep writing");
  assert.equal(waitingLabel(false, 2), "Working · 2 background tasks · you can keep writing");
  assert.equal(waitingLabel(true, 0), "Waiting for background tasks · you can keep writing");
});

test("a follow-up that moved below a reply replaces its earlier row", () => {
  const messages = [
    { id: 1, role: "user", content: "build", turn_id: 7 },
    { id: 2, role: "user", content: "status?", turn_id: 0 },
    { id: 3, role: "assistant", content: "Game A built", turn_id: 7 },
  ];
  takeFollowUp(messages, { id: 4, role: "user", content: "status?", turn_id: 7 }, 2);
  assert.deepEqual(messages.map((m) => m.id), [1, 3, 4]);
  // One that kept its id is updated in place.
  takeFollowUp(messages, { id: 4, role: "user", content: "status?", turn_id: 7 });
  assert.deepEqual(messages.map((m) => m.id), [1, 3, 4]);
});

test("a stored prompt without a turn is waiting while held, else unprocessed", () => {
  assert.equal(promptState({ id: 5, role: "user", turn_id: 0 }, [5]), "waiting");
  assert.equal(promptState({ id: 5, role: "user", turn_id: 0 }, []), "unprocessed");
  assert.equal(promptState({ id: 5, role: "user" }), "unprocessed");
  assert.equal(promptState({ id: 5, role: "user", turn_id: 9 }), "");
  assert.equal(promptState({ role: "user", content: "just sent" }), "");
  assert.equal(promptState({ role: "user", content: "joined", waiting: true }), "waiting");
  assert.equal(promptState({ id: 6, role: "assistant", turn_id: 0 }), "");
});

test("a spawned task's copies of its thread's prompts ran there", () => {
  assert.equal(promptState({ id: 7, role: "user", created_at: 100 }, [], 200), "");
  assert.equal(promptState({ id: 8, role: "user", created_at: 200 }, [], 200), "unprocessed");
});
