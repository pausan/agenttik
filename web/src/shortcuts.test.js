import test from "node:test";
import assert from "node:assert/strict";

import { ACTIONS, actionOf, matches } from "./shortcuts.js";
import { primaryChord } from "./platform.js";

/* The new-terminal chord — see specs/073-terminals.md. It shares its letter
   with the Tree panel (Alt+T), a new task (Ctrl+T) and a reopened tab
   (Ctrl+Shift+T), so what keeps the four apart is that matches() is exact on
   the modifiers. */
const pressingT = (mods) => ({ code: "KeyT", ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, ...mods });

test("a new terminal is Ctrl+Alt+T, and the other T chords are not it", () => {
  assert.deepEqual(actionOf("terminal.new").keys, [primaryChord("Alt+T")]);

  const chord = pressingT({ ctrlKey: true, altKey: true });
  assert.equal(matches(chord, "Ctrl+Alt+T"), true);
  assert.equal(matches(chord, "Alt+T"), false);
  assert.equal(matches(chord, "Ctrl+T"), false);
  assert.equal(matches(chord, "Ctrl+Shift+T"), false);

  // And none of theirs opens a terminal.
  assert.equal(matches(pressingT({ altKey: true }), "Ctrl+Alt+T"), false);
  assert.equal(matches(pressingT({ ctrlKey: true }), "Ctrl+Alt+T"), false);
  assert.equal(matches(pressingT({ ctrlKey: true, shiftKey: true }), "Ctrl+Alt+T"), false);
  assert.equal(matches(pressingT({ ctrlKey: true, altKey: true, shiftKey: true }), "Ctrl+Alt+T"), false);
});

test("no other action is bound to the new-terminal chord", () => {
  const taken = ACTIONS.filter((a) => a.id !== "terminal.new" && a.keys.includes(primaryChord("Alt+T")));
  assert.deepEqual(taken.map((a) => a.id), []);
});

test("macOS spells it Cmd+Alt+T", () => {
  assert.equal(primaryChord("Alt+T", true), "Cmd+Alt+T");
  const chord = pressingT({ altKey: true, metaKey: true });
  assert.equal(matches(chord, "Cmd+Alt+T"), true);
  assert.equal(matches(chord, "Ctrl+Alt+T"), false);
});
