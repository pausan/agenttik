import test, { mock } from "node:test";
import assert from "node:assert/strict";
import { LEVELS, base64PCM, createDictation, loudness, nativeMicrophone, pcm16, rms } from "./speech-recorder.js";

test("PCM averages downsampled audio, clamps and writes little endian", () => {
  const samples = pcm16([new Float32Array([1, 1, 1, -1, -1, -1, 2, 2, 2])], 48000);
  const view = new DataView(samples.buffer);
  assert.equal(samples.length, 6);
  assert.deepEqual([0, 2, 4].map((i) => view.getInt16(i, true)), [32767, -32768, 32767]);
  assert.equal(base64PCM(samples), Buffer.from(samples).toString("base64"));
  assert.equal(pcm16([], 44100).length, 0);
});

test("loudness maps RMS to 0..1 on a decibel scale", () => {
  assert.equal(rms(new Float32Array([0.5, -0.5])), 0.5);
  assert.equal(rms(new Float32Array()), 0);
  assert.deepEqual([0, 0.001, 0.0178, 0.316, 1].map((v) => Math.round(loudness(v) * 100) / 100), [0, 0, 0.44, 1, 1]);
});

function fixture(overrides = {}) {
  const state = { phase: "idle" }, calls = [], delivered = [], errors = [], warnings = [];
  const mic = { started: false, cancelled: false, start(_onLimit, onLevel) { this.started = true; this.onLevel = onLevel; }, stop() { return "AAA="; }, cancel() { this.cancelled = true; } };
  const controller = createDictation({
    state, capture: async () => mic,
    request: async (method, path, body) => {
      calls.push({ method, path, body });
      if (path === "/api/speech") return { model: "whistle", ready: false };
      if (path.endsWith("transcribe")) return { text: "Hola mundo.", warning: "" };
    },
    deliver: (owner, text) => delivered.push({ owner, text }),
    onError: (error) => errors.push(error.message), onWarning: (warning) => warnings.push(warning),
    ...overrides,
  });
  return { controller, state, calls, delivered, errors, warnings, mic };
}

test("first use prepares once, records, then transcribes in one request to the captured draft", async () => {
  const f = fixture(), owner = { draft: "existing" };
  await f.controller.toggle(owner);
  assert.equal(f.state.phase, "recording");
  assert.equal(f.mic.started, true);
  await f.controller.toggle({ draft: "another tab" });
  assert.equal(f.state.phase, "idle");
  assert.deepEqual(f.calls.map((c) => c.path), ["/api/speech", "/api/speech/prepare", "/api/speech/transcribe"]);
  assert.deepEqual(f.calls[2].body, { model: "whistle", pcm: "AAA=" });
  assert.deepEqual(f.delivered, [{ owner, text: "Hola mundo." }]);
});

test("recording keeps the latest levels for the meter and ignores late ones", async () => {
  const f = fixture();
  await f.controller.toggle({});
  assert.deepEqual(f.state.levels, []);
  for (let i = 0; i < LEVELS + 2; i++) f.mic.onLevel(i);
  assert.equal(f.state.levels.length, LEVELS);
  assert.equal(f.state.levels[0], 2);
  assert.equal(f.state.levels.at(-1), LEVELS + 1);
  await f.controller.stop();
  f.mic.onLevel(99);
  assert.notEqual(f.state.levels.at(-1), 99);
});

test("download spinner remains visible; cancelling prevents recording or late insertion", async () => {
  let ready;
  const pending = new Promise((resolve) => { ready = resolve; });
  const f = fixture({ request: async (_method, path) => path === "/api/speech" ? { model: "whistle", ready: false } : pending });
  const start = f.controller.toggle({});
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(f.state.phase, "downloading");
  f.controller.cancel(); ready(); await start;
  assert.equal(f.state.phase, "idle");
  assert.equal(f.mic.started, false);
  assert.equal(f.mic.cancelled, true);
  assert.deepEqual(f.delivered, []);
});

test("cancelled microphone permission is cleaned up when it eventually resolves", async () => {
  let allow;
  const f = fixture({ capture: () => new Promise((resolve) => { allow = resolve; }) });
  const start = f.controller.toggle({});
  f.controller.cancel(); allow(f.mic); await start;
  assert.equal(f.mic.cancelled, true);
  assert.deepEqual(f.calls, []);
});

test("failed transcription releases recording and permits retry", async () => {
  const f = fixture({ request: async (_method, path) => {
    if (path === "/api/speech") return { model: "whisper-base", ready: true };
    if (path.endsWith("transcribe")) throw new Error("Could not transcribe");
  } });
  await f.controller.toggle({}); await f.controller.stop();
  assert.equal(f.state.phase, "idle");
  assert.deepEqual(f.errors, ["Could not transcribe"]);
  assert.deepEqual(f.delivered, []);
});

test("silence does not insert text, and correction failure retains original transcript", async () => {
  for (const text of ["", "Raw transcript"]) {
    const f = fixture({ request: async (_method, path) => path === "/api/speech" ? { model: "whistle", ready: true }
      : path.endsWith("transcribe") ? { text, warning: "Original transcript kept" } : null });
    await f.controller.toggle({}); await f.controller.stop();
    if (text) { assert.equal(f.delivered[0].text, text); assert.equal(f.warnings.length, 1); }
    else { assert.equal(f.delivered.length, 0); assert.match(f.errors[0], /No speech detected/); }
  }
});

test("native microphone is absent outside the mobile app", () => {
  assert.equal(nativeMicrophone({}), null);
});

test("Android native microphone matches replies by id and reports errors", async () => {
  const sent = [];
  const bridge = { postMessage: (text) => sent.push(JSON.parse(text)) };
  const reply = (id, body) => bridge.onmessage({ data: JSON.stringify({ id, ...body }) });
  const opening = nativeMicrophone({ agenttikMicrophone: bridge })();
  await Promise.resolve();
  assert.deepEqual(sent, [{ id: 1, command: "permission" }]);
  reply(1, {});
  const mic = await opening;
  mic.start(() => {});
  const stopping = mic.stop();
  assert.deepEqual(sent.slice(1).map((m) => m.command), ["start", "stop"]);
  reply(3, { data: "AAA=" }); reply(2, {});
  assert.equal(await stopping, "AAA=");

  const denied = nativeMicrophone({ agenttikMicrophone: bridge })();
  await Promise.resolve();
  reply(sent.at(-1).id, { error: "Allow microphone access" });
  await assert.rejects(denied, /Allow microphone access/);
});

test("iOS native microphone uses the handler's promise", async () => {
  const commands = [];
  const handler = { postMessage: async ({ command }) => { commands.push(command); return command === "stop" ? "AAA=" : null; } };
  const mic = await nativeMicrophone({ webkit: { messageHandlers: { agenttikMicrophone: handler } } })();
  mic.start(() => {});
  assert.equal(await mic.stop(), "AAA=");
  assert.deepEqual(commands, ["permission", "start", "stop"]);
});

test("native microphone polls the level while recording only", async () => {
  mock.timers.enable({ apis: ["setInterval", "setTimeout"] });
  try {
    const commands = [], levels = [];
    const handler = { postMessage: async ({ command }) => { commands.push(command); return command === "level" ? 0.316 : null; } };
    const mic = await nativeMicrophone({ webkit: { messageHandlers: { agenttikMicrophone: handler } } })();
    mic.start(() => {}, (level) => levels.push(level));
    mock.timers.tick(100);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(levels.length, 1);
    assert.ok(levels[0] > 0.99);
    mic.cancel();
    mock.timers.tick(500);
    assert.deepEqual(commands, ["permission", "start", "level", "cancel"]);
  } finally { mock.timers.reset(); }
});
