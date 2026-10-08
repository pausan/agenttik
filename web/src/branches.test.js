import { test } from "node:test";
import assert from "node:assert/strict";
import { branchOptions } from "./branches.js";

test("local branches precede remote groups and keep distinct selection values", () => {
  const groups = branchOptions(["main", "topic/path"], [
    { remote: "origin", branch: "main" }, { remote: "origin", branch: "topic/path" },
    { remote: "other", branch: "main" },
  ]);
  assert.deepEqual(groups.map(group => group[0]), [
    { type: "label", label: "Local" }, { type: "label", label: "origin" }, { type: "label", label: "other" },
  ]);
  assert.deepEqual(groups[1][2], { label: "origin/topic/path", value: "refs/remotes/origin/topic/path", branch: "topic/path", remote: "origin" });
  assert.equal(new Set(groups.flat().filter(item => item.value).map(item => item.value)).size, 5);
});

test("search ranks within each group and drops groups with no matches", () => {
  const groups = branchOptions(["topic/path"], [
    { remote: "origin", branch: "main" }, { remote: "origin", branch: "topic/path" },
    { remote: "other", branch: "main" },
  ], " topic ");
  assert.deepEqual(groups.map(group => group.map(item => item.label)), [["Local", "topic/path"], ["origin", "origin/topic/path"]]);
  assert.deepEqual(branchOptions([], [], "main"), []);
});
