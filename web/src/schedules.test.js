import assert from "node:assert/strict";
import test from "node:test";
import { DEFAULT_SCHEDULE, scheduleForm, scheduleLabel, scheduleRecurrence } from "./schedules.js";

test("calendar recurrences round trip through the form and show the chosen clock", () => {
  for (const [schedule, label] of [
    [{ every: "days", day_interval: 3, weekday_mask: 0, at_minute: 570, interval_minutes: 0 }, "Every 3 days at 09:30"],
    [{ every: "weekdays", day_interval: 1, weekday_mask: 42, at_minute: 1085, interval_minutes: 0 }, "Mon, Wed, Fri at 18:05"],
    [{ every: "day", day_interval: 1, weekday_mask: 0, at_minute: 540, interval_minutes: 0 }, "Every day at 09:00"],
    [{ every: "interval", day_interval: 1, weekday_mask: 0, at_minute: 0, interval_minutes: 90 }, "Every 1h 30m"],
  ]) {
    assert.deepEqual(scheduleRecurrence(scheduleForm(schedule)), schedule);
    assert.equal(scheduleLabel(schedule), label);
  }
});

test("switching calendar forms offers usable defaults", () => {
  const form = scheduleForm({ every: "interval", interval_minutes: 15 });
  assert.equal(form.at, "09:00");
  assert.equal(form.days, 2);
  assert.equal(form.weekdayMask, 62);
  assert.deepEqual(scheduleRecurrence({ ...form, every: "weekdays" }), {
    every: "weekdays", interval_minutes: 0, at_minute: 540, day_interval: 1, weekday_mask: 62,
  });
});

test("day counts are whole, bounded positive numbers", () => {
  for (const [days, want] of [[0, 1], [-1, 1], ["", 1], ["3", 3], [2.5, 2], [100000, 36500]]) {
    assert.equal(scheduleRecurrence({ ...DEFAULT_SCHEDULE, every: "days", days }).day_interval, want);
  }
});
