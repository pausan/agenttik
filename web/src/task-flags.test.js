import test from "node:test";
import assert from "node:assert/strict";
import { nextFlag, taskFlag } from "./task-flags.js";

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
