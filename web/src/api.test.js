import { strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { isoDate } from "./api.js";

test("stats dates use a locale-independent ISO timestamp", () => {
  strictEqual(isoDate(Date.UTC(2026, 8, 12, 13, 32, 11)), "2026-09-12 13:32:11");
  strictEqual(isoDate(0), "never");
});
