import test from "node:test";
import assert from "node:assert/strict";
import { nextDot, nextFlag, taskFlag } from "./task-flags.js";

test("flags cycle from none through star, warning and danger back to none", () => {
  const seen = [];
  let flag = "";
  for (let i = 0; i < 4; i++) seen.push(flag = nextFlag(flag));
  assert.deepEqual(seen, ["star", "warn", "danger", ""]);
});

test("an unknown or missing flag is no flag", () => {
  assert.equal(nextFlag(undefined), "star");
  assert.equal(taskFlag(undefined), null);
  assert.equal(taskFlag("warn").icon, "i-lucide-triangle-alert");
});

test("a read, unmarked dot marks unread first and the cycle ends read", () => {
  const seen = [];
  let dot = { flag: "", unread: false };
  for (let i = 0; i < 5; i++) seen.push(dot = nextDot(dot.flag, dot.unread));
  assert.deepEqual(seen, [
    { flag: "", unread: true },
    { flag: "star", unread: true },
    { flag: "warn", unread: true },
    { flag: "danger", unread: true },
    { flag: "", unread: false },
  ]);
});

test("an unread or marked dot moves straight to the next mark", () => {
  assert.deepEqual(nextDot("", true), { flag: "star", unread: true });
  assert.deepEqual(nextDot("warn", false), { flag: "danger", unread: true });
});
