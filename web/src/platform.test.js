import test from "node:test";
import assert from "node:assert/strict";

import { desktopChord, isMacOS, needsCSSScrollbars, platformChord, primaryChord } from "./platform.js";
import { matches } from "./shortcuts.js";

test("CSS scrollbars are limited to Linux desktop windows", () => {
  assert.equal(needsCSSScrollbars("Linux x86_64", {}), true);
  assert.equal(needsCSSScrollbars("Linux aarch64", {}), true);
  assert.equal(needsCSSScrollbars("Linux x86_64", null), false);
  assert.equal(needsCSSScrollbars("MacIntel", {}), false);
  assert.equal(needsCSSScrollbars("Win32", {}), false);
});

test("macOS uses Cmd spelling for legacy Meta chords", () => {
  assert.equal(isMacOS("MacIntel"), true);
  assert.equal(isMacOS("Win32"), false);
  assert.equal(primaryChord("Shift+P", true), "Cmd+Shift+P");
  assert.equal(primaryChord("Shift+P", false), "Ctrl+Shift+P");
  assert.equal(platformChord("Meta+Shift+P", true), "Cmd+Shift+P");
  assert.equal(platformChord("Ctrl+Shift+P", true), "Ctrl+Shift+P");
  assert.equal(desktopChord("Ctrl+Shift+A", true), "Cmd+Shift+A");
});

test("Cmd chords match the macOS Command modifier", () => {
  const event = { code: "KeyP", ctrlKey: false, altKey: false, shiftKey: true, metaKey: true };
  assert.equal(matches(event, "Cmd+Shift+P"), true);
  assert.equal(matches(event, "Ctrl+Shift+P"), false);
});
