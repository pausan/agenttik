import { test } from "node:test";
import assert from "node:assert/strict";
import { messageDate, messageTime } from "./message-time.js";

test("message times are a 24-hour clock with seconds", () => {
  assert.equal(messageTime(new Date(2026, 9, 23, 12, 32, 44).getTime()), "12:32:44");
  assert.equal(messageTime(new Date(2026, 9, 23, 0, 5, 9).getTime()), "00:05:09");
  assert.equal(messageTime(new Date(2026, 9, 23, 18, 0, 0).getTime()), "18:00:00");
});

test("the tooltip names the day as well as the time", () => {
  const full = messageDate(new Date(2026, 9, 23, 12, 32, 44).getTime());
  assert.match(full, /2026/);
  assert.match(full, /12:32:44/);
});

test("a message without a time shows nothing", () => {
  assert.equal(messageTime(0), "");
  assert.equal(messageTime(undefined), "");
  assert.equal(messageDate(undefined), "");
});
