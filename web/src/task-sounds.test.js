import test from "node:test";
import assert from "node:assert/strict";
import { createTaskSounds, TASK_SOUNDS_KEY } from "./task-sounds.js";

function fixture(saved) {
  const values = new Map(saved === undefined ? [] : [[TASK_SOUNDS_KEY, saved]]);
  const calls = { contexts: 0, tones: 0, stops: 0, disconnects: 0 };
  class AudioContext {
    state = "running";
    currentTime = 0;
    constructor() { calls.contexts++; }
    createOscillator() {
      calls.tones++;
      return { frequency: {}, connect() {}, disconnect() { calls.disconnects++; },
        start() {}, stop() { calls.stops++; this.onended(); } };
    }
    createGain() {
      return { gain: { setValueAtTime() {}, linearRampToValueAtTime() {}, exponentialRampToValueAtTime() {} },
        connect() {}, disconnect() { calls.disconnects++; } };
    }
  }
  const sounds = createTaskSounds({ getItem: (k) => values.get(k), setItem: (k, v) => values.set(k, v) }, AudioContext);
  sounds.load();
  return { sounds, calls, values };
}
const done = (turn = 1) => ({ session_id: 1, turn_id: turn, event: { type: "done" }, stats: {} });

test("defaults off without allocating audio, saves opt-in and disabling silences events", () => {
  const { sounds, calls, values } = fixture();
  assert.equal(sounds.enabled.value, false);
  sounds.unlock();
  sounds.onEvent(done());
  assert.equal(calls.contexts, 0);
  sounds.setEnabled(true);
  assert.equal(values.get(TASK_SOUNDS_KEY), "1");
  sounds.onEvent(done(2));
  assert.equal(calls.tones, 1);
  assert.equal(calls.stops, 1);
  assert.equal(calls.disconnects, 2);
  sounds.setEnabled(false);
  sounds.onEvent(done(3));
  assert.equal(calls.tones, 1);
  assert.equal(values.get(TASK_SOUNDS_KEY), "0");
});

test("restores only explicit opt-in and ignores streaming or unfinished done events", () => {
  for (const value of ["0", "true", "", "invalid"]) assert.equal(fixture(value).sounds.enabled.value, false);
  const { sounds, calls } = fixture("1");
  assert.equal(sounds.enabled.value, true);
  assert.equal(calls.contexts, 0);
  sounds.onEvent({ ...done(), stats: undefined });
  sounds.onEvent({ ...done(), event: { type: "text" } });
  assert.equal(calls.tones, 0);
  sounds.onEvent(done());
  assert.equal(calls.tones, 1);
});

test("errors and completions ding once per turn despite interleaved duplicate deliveries", () => {
  const { sounds, calls } = fixture("1");
  const error = { ...done(), event: { type: "error" }, stats: undefined };
  sounds.onEvent(error);
  sounds.onEvent(done(2));
  sounds.onEvent(error);
  sounds.onEvent(done());
  sounds.onEvent(done(2));
  assert.equal(calls.tones, 2);
  sounds.onEvent({ ...done(), session_id: 2 });
  assert.equal(calls.tones, 3);
});

test("unavailable audio and storage cannot interrupt task handling", () => {
  const sounds = createTaskSounds({ getItem() { throw Error(); }, setItem() { throw Error(); } }, class {
    constructor() { throw Error("Unavailable"); }
  });
  assert.doesNotThrow(() => { sounds.load(); sounds.setEnabled(true); sounds.onEvent(done()); });
});
