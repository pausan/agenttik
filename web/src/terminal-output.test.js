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
});
