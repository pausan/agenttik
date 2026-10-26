import { deepStrictEqual, strictEqual } from "node:assert/strict";
import { test } from "node:test";
import { terminalOutput } from "./terminal-output.js";

function emulator() {
  const queries = new Map();
  let input;
  const register = (kind, id, handler) => {
    const key = JSON.stringify([kind, id]);
    queries.set(key, handler);
    return { dispose: () => queries.delete(key) };
  };
  return {
    writes: [],
    resets: 0,
    bottomScrolls: 0,
    parser: {
      registerCsiHandler: (id, handler) => register("csi", id, handler),
      registerOscHandler: (id, handler) => register("osc", id, handler),
      registerDcsHandler: (id, handler) => register("dcs", id, handler),
    },
    onData(handler) {
      input = handler;
      return { dispose: () => { input = null; } };
    },
    input(data) { input?.(data); },
    query(kind, id, reply) {
      // A false return lets xterm perform its normal state updates and reply.
      strictEqual(queries.get(JSON.stringify([kind, id]))(), false);
      input?.(reply);
    },
    erase(mode) { strictEqual(queries.get(JSON.stringify(["csi", { final: "J" }]))([mode]), false); },
    scrollToBottom() { this.bottomScrolls++; },
    reset() { this.resets++; },
    write(bytes, callback) { this.writes.push({ bytes, callback }); },
  };
}

test("replayed queries are silent while typed and pasted input still reaches the shell", async () => {
  const term = emulator();
  const sent = [];
  const output = terminalOutput(term, data => sent.push(data));
  const done = output.write("history", false, true);
  await Promise.resolve();
  for (const [kind, id, reply] of [
    ["csi", { final: "n" }, "\x1b[2;1R"],
    ["csi", { prefix: ">", final: "c" }, "\x1b[>0;276;0c"],
    ["csi", { intermediates: "$", final: "p" }, "\x1b[12;2$y"],
    ["osc", 11, "\x1b]11;rgb:1818/1818/1b1b\x1b\\"],
    ["dcs", { intermediates: "$", final: "q" }, "\x1bP1$r0m\x1b\\"],
  ]) {
    term.query(kind, id, reply);
  }
  deepStrictEqual(sent, []);
  // Input events run between parser slices, before the replay has finished.
  await Promise.resolve();
  term.input("typed");
  term.input("\x1b[200~pasted\x1b[201~");
  deepStrictEqual(sent, ["typed", "\x1b[200~pasted\x1b[201~"]);
  term.writes[0].callback();
  await done;
  output.dispose();
});

test("clearing scrollback resumes following output without replacing erase handling", async () => {
  const term = emulator();
  const output = terminalOutput(term, () => {});
  term.erase(0);
  term.erase(1);
  term.erase(2);
  strictEqual(term.bottomScrolls, 0);
  term.erase(3);
  strictEqual(term.bottomScrolls, 0);
  await Promise.resolve();
  strictEqual(term.bottomScrolls, 1);
  term.erase(3);
  output.dispose();
  await Promise.resolve();
  strictEqual(term.bottomScrolls, 1);
});

test("output after a scrollback clear stays at the bottom until it settles", async () => {
  const term = emulator();
  const output = terminalOutput(term, () => {});
  output.write("clear", false, false);
  term.erase(3);
  await Promise.resolve();
  strictEqual(term.bottomScrolls, 1);
  // The listing that follows the clear keeps the bottom in view as it draws.
  output.write("seq", false, false);
  term.writes[0].callback();
  await new Promise(resolve => setTimeout(resolve));
  strictEqual(term.bottomScrolls, 2);
  term.writes[1].callback();
  await new Promise(resolve => setTimeout(resolve));
  strictEqual(term.bottomScrolls, 3);
  // Two frames after the output stops, once more, and then it is left alone.
  await new Promise(resolve => setTimeout(resolve, 50));
  strictEqual(term.bottomScrolls, 4);
  output.write("later", false, false);
  term.writes[2].callback();
  await new Promise(resolve => setTimeout(resolve, 50));
  strictEqual(term.bottomScrolls, 4);
});

test("queued live queries keep their replies and resets wait for previous writes", async () => {
  const term = emulator();
  const sent = [];
  let drawn = 0;
  const output = terminalOutput(term, data => sent.push(data), () => drawn++);
  output.write("history", false, true);
  const done = output.write("live", true, false);
  await Promise.resolve();
  strictEqual(term.writes.length, 1);
  strictEqual(term.resets, 0);
  term.query("osc", 10, "old color");
  term.writes[0].callback();
  await new Promise(resolve => setImmediate(resolve));
  strictEqual(term.resets, 1);
  term.query("osc", 10, "live color");
  term.query("csi", { final: "n" }, "live position");
  term.writes[1].callback();
  await done;
  deepStrictEqual(sent, ["live color", "live position"]);
  strictEqual(drawn, 2);
  output.dispose();
});

test("disposing a view discards queued frames and pending draw callbacks", async () => {
  const term = emulator();
  let drawn = 0;
  const output = terminalOutput(term, () => {}, () => drawn++);
  output.write("history", false, true);
  const done = output.write("queued", true, false);
  await Promise.resolve();
  output.dispose();
  term.writes[0].callback();
  await done;
  strictEqual(term.writes.length, 1);
  strictEqual(term.resets, 0);
  strictEqual(drawn, 0);
  await output.write("after disposal", false, false);
  strictEqual(term.writes.length, 1);
});

test("a burst joins queued bytes in order without corrupting split UTF-8 or escapes", async () => {
  const term = emulator();
  const output = terminalOutput(term, () => {});
  output.write(new Uint8Array([0]), false, false);
  const chunks = [[0xe2], [0x82, 0xac, 0x1b], [0x5b, 0x33, 0x31], [0x6d]];
  for (const chunk of chunks) output.write(new Uint8Array(chunk), false, false);
  const done = output.write(new Uint8Array([0xff]), false, false);
  strictEqual(term.writes.length, 1);
  term.writes[0].callback();
  await new Promise(resolve => setImmediate(resolve));
  strictEqual(term.writes.length, 2);
  deepStrictEqual([...term.writes[1].bytes], [...chunks.flat(), 0xff]);
  term.writes[1].callback();
  await done;

  // Once idle, a new write starts another drain without waiting for a batch.
  const next = output.write(new Uint8Array([1]), false, false);
  strictEqual(term.writes.length, 3);
  term.writes[2].callback();
  await next;
  output.dispose();
});

test("batches stop at replay changes and resets, preserving query replies", async () => {
  const term = emulator();
  const sent = [];
  const output = terminalOutput(term, data => sent.push(data));
  const write = (byte, reset, replay) => output.write(new Uint8Array([byte]), reset, replay);
  write(0, false, true);
  write(1, false, true);
  write(2, false, true);
  write(3, false, false);
  write(4, false, false);
  write(5, true, false);
  const done = write(6, false, false);
  for (const [i, expected, resets] of [[0, [0], 0], [1, [1, 2], 0], [2, [3, 4], 0], [3, [5, 6], 1]]) {
    deepStrictEqual([...term.writes[i].bytes], expected);
    strictEqual(term.resets, resets);
    term.query("csi", { final: "n" }, `reply ${i}`);
    term.writes[i].callback();
    await new Promise(resolve => setImmediate(resolve));
  }
  await done;
  deepStrictEqual(sent, ["reply 2", "reply 3"]);
  output.dispose();
});

test("large backlogs use bounded batches instead of one blocking write", async () => {
  const term = emulator();
  const output = terminalOutput(term, () => {});
  output.write(new Uint8Array([0]), false, false);
  for (let i = 0; i < 10; i++) {
    output.write(new Uint8Array(32 << 10).fill(i + 1), false, false);
  }
  const done = output.write(new Uint8Array([11]), false, false);
  for (let i = 0; i < 4; i++) {
    strictEqual(term.writes.length, i + 1);
    strictEqual(term.writes[i].bytes.length <= 128 << 10, true);
    term.writes[i].callback();
    await new Promise(resolve => setImmediate(resolve));
  }
  await done;
  const bytes = term.writes.flatMap(write => [...write.bytes]);
  strictEqual(bytes.length, (320 << 10) + 2);
  strictEqual(bytes[0], 0);
  for (let i = 0; i < 10; i++) {
    strictEqual(bytes[1 + (i * (32 << 10))], i + 1);
    strictEqual(bytes[(i + 1) * (32 << 10)], i + 1);
  }
  strictEqual(bytes.at(-1), 11);
  output.dispose();
});
