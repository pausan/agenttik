import test from "node:test";
import assert from "node:assert/strict";
import { connectedAccounts, insertTourPrompt, TOUR_STEPS } from "./quick-start.js";
import { readTour, saveTour, TOUR_KEY } from "./quick-start-state.js";

function memory() {
  const data = new Map();
  return { getItem: (key) => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) };
}

test("the first empty workspace opens the tour, existing workspaces opt in", () => {
  assert.equal(readTour(memory(), true).open, true);
  assert.equal(readTour(memory(), false).open, false);
});

test("progress resumes and dismissal persists even in an empty workspace", () => {
  const storage = memory();
  saveTour(storage, { open: true, step: "divide" });
  assert.deepEqual(readTour(storage, false), { open: true, step: "divide" });
  saveTour(storage, { open: false, step: "divide" });
  assert.equal(readTour(storage, true).open, false);
  storage.setItem(TOUR_KEY, "broken");
  assert.deepEqual(readTour(storage, true), { open: true, step: "welcome" });
  const denied = { getItem() { throw new Error(); }, setItem() { throw new Error(); } };
  assert.equal(readTour(denied, true).open, true);
  assert.doesNotThrow(() => saveTour(denied, { open: false }));
});

test("copying fills only an empty draft and preserves pending images and text", () => {
  const owner = { draft: "" };
  assert.equal(insertTourPrompt(owner, "hello"), true);
  assert.equal(owner.draft, "hello");
  assert.equal(insertTourPrompt(owner, "replacement"), false);
  assert.equal(owner.draft, "hello");
  assert.equal(insertTourPrompt({ draft: "", imageUploads: 1 }, "hello"), false);
  assert.equal(insertTourPrompt(null, "hello"), false);
});

test("connection status needs both an available provider and a signed-in account", () => {
  assert.deepEqual(connectedAccounts([
    { available: true, display_name: "Codex", accounts: [{ alias: "Work", signed_in: true }, { alias: "Other", signed_in: false }] },
    { available: false, display_name: "Claude", accounts: [{ alias: "Home", signed_in: true }] },
    { available: true, display_name: "Empty" },
  ]), ["Codex · Work"]);
});

test("calculator prompts leave time to enqueue and request local commits", () => {
  for (const id of ["instructions", "add", "multiply", "divide"]) {
    const { prompt } = TOUR_STEPS.find((step) => step.id === id);
    assert.match(prompt, /time.sleep\(30\)/);
    assert.match(prompt, /commit only this task's changes locally/);
    assert.match(prompt, /Do not push/);
  }
});
