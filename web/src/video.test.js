import { strictEqual } from "node:assert/strict";
import { test } from "node:test";
import { formatTime, inRanges, SPEEDS, stepSpeed } from "./video.js";

test("times read as m:ss, h:mm:ss past an hour, and dashes when unknown", () => {
  for (const [seconds, expected] of [
    [0, "0:00"], [5.9, "0:05"], [65, "1:05"], [599, "9:59"], [3600, "1:00:00"],
    [3725, "1:02:05"], [NaN, "–:––"], [Infinity, "–:––"], [-1, "–:––"],
  ]) strictEqual(formatTime(seconds), expected);
});

test("speed steps stop at both ends and snap an odd rate to the next step", () => {
  strictEqual(stepSpeed(1, 1), 1.25);
  strictEqual(stepSpeed(1, -1), 0.75);
  strictEqual(stepSpeed(SPEEDS.at(-1), 1), SPEEDS.at(-1));
  strictEqual(stepSpeed(SPEEDS[0], -1), SPEEDS[0]);
  strictEqual(stepSpeed(1.1, 1), 1.25);
  strictEqual(stepSpeed(1.1, -1), 1);
});

test("a time is inside a TimeRanges only within one of its ranges", () => {
  const list = [[0, 5], [10, 30]];
  const ranges = { length: list.length, start: (i) => list[i][0], end: (i) => list[i][1] };
  strictEqual(inRanges(ranges, 12), true);
  strictEqual(inRanges(ranges, 10), true);
  strictEqual(inRanges(ranges, 7), false);
  strictEqual(inRanges(ranges, 31), false);
  strictEqual(inRanges({ length: 0 }, 0), false);
});
