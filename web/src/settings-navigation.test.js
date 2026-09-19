import test from "node:test";
import assert from "node:assert/strict";
import { SETTINGS_SECTIONS, visibleSettings } from "./settings-navigation.js";

test("settings preserve the requested hierarchy and order", () => {
  assert.deepEqual(SETTINGS_SECTIONS.map((s) => s.label), ["General", "Orchestrator", "Providers", "Models", "Server", "Help", "About"]);
  assert.deepEqual(SETTINGS_SECTIONS[0].children.map((s) => s.label), ["Appearance", "Profiles", "Projects", "Shortcuts"]);
  assert.deepEqual(SETTINGS_SECTIONS[2].children.map((s) => s.label), ["Subscriptions", "API Providers"]);
});
test("filtering retains ancestors and counts matching descendants", () => {
  const sections = visibleSettings("key", { shortcuts: 2, "api-providers": 8, models: 0 });
  assert.deepEqual(sections.map((s) => [s.id, s.count]), [["general", 2], ["providers", 8]]);
  assert.deepEqual(sections[0].children.map((s) => s.id), ["shortcuts"]);
  assert.deepEqual(visibleSettings("nothing", {}), []);
});
