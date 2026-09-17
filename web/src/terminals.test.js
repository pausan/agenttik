import { deepStrictEqual, ok, strictEqual } from "node:assert/strict";
import { test } from "node:test";

/* The terminal client's two careful parts: keystrokes must reach the shell in
   the order they were typed, and a stream reopened because some *other*
   terminal was opened must not redraw this one's output over itself. */

/* One fake stream per open, so a test can push frames and see what was asked
   for. */
class FakeEventSource {
  static opened = [];
  constructor(url) {
    this.url = url;
    this.onmessage = null;
    this.onerror = null;
    FakeEventSource.opened.push(this);
  }
  push(frame) {
    this.onmessage?.({ data: JSON.stringify(frame) });
  }
  close() {
    this.closed = true;
  }
}
globalThis.EventSource = FakeEventSource;

const { resizeTerminal, sendTerminalBinary, sendTerminalInput, watchTerminal } =
  await import("./terminals.js");

/* calls collects every request, and lets a test hold one open so it can type
   while a POST is still in flight. */
function captureFetch() {
  const calls = [];
  let release;
  const held = new Promise((resolve) => (release = resolve));
  globalThis.fetch = async (url, options) => {
    const body = options?.body;
    const text = body instanceof Blob ? await body.text() : String(body ?? "");
    calls.push({ url: String(url), body: text });
    if (calls.length === 1) await held;
    return new Response("", { status: 204 });
  };
  return { calls, release: () => release() };
}

test("keystrokes reach the shell in the order they were typed", async () => {
  const { calls, release } = captureFetch();

  // The first send goes out alone and is held open.
  sendTerminalInput("order", "a");
  await new Promise((r) => setImmediate(r));
  strictEqual(calls.length, 1, "the first keystroke should be sent at once");
  strictEqual(calls[0].body, "a");

  // Everything typed while it is in flight waits, and travels together in the
  // order it was typed rather than racing the one already out.
  sendTerminalInput("order", "b");
  sendTerminalInput("order", "c");
  sendTerminalInput("order", "d");
  strictEqual(calls.length, 1, "a second request must not overlap the first");

  release();
  await new Promise((r) => setTimeout(r, 10));
  strictEqual(calls.length, 2, "what was typed during the flight is one request");
  strictEqual(calls[1].body, "bcd");
  ok(calls[1].url.includes("/api/terminals/order/input"));
});

test("report-mode bytes are not widened into text", async () => {
  const { calls, release } = captureFetch();
  release();

  // 0xe9 is one byte the shell must receive as one byte. Sent as text it
  // would arrive as the two bytes UTF-8 spells it with.
  sendTerminalBinary("bytes", "é");
  await new Promise((r) => setTimeout(r, 10));
  strictEqual(calls.length, 1);
  strictEqual(calls[0].body.length, 1, "a report byte should stay one byte");
});

test("a reopened stream resumes each terminal where its view got to", async () => {
  const { release } = captureFetch();
  release();
  FakeEventSource.opened.length = 0;

  const drawn = [];
  const stopFirst = watchTerminal("first", {
    onData: (bytes, reset) => drawn.push({ text: Buffer.from(bytes).toString(), reset }),
  });
  const stream = FakeEventSource.opened.at(-1);
  ok(stream.url.includes("first%3A0"), "a view with nothing asks from zero");

  // 512 bytes have reached this view, of which the frame carries the last 5.
  stream.push({ id: "first", data: Buffer.from("hello").toString("base64"), at: 512 });
  deepStrictEqual(drawn, [{ text: "hello", reset: false }]);

  // Opening a second terminal reopens the shared stream. The first must be
  // resumed from where it got to, or its screen would be drawn again on top
  // of itself.
  const stopSecond = watchTerminal("second", { onData: () => {} });
  const reopened = FakeEventSource.opened.at(-1);
  ok(stream.closed, "the old stream is closed rather than left running");
  ok(reopened.url.includes("first%3A512"), `resumed from the wrong place: ${reopened.url}`);
  ok(reopened.url.includes("second%3A0"), `the new terminal was not asked for: ${reopened.url}`);

  // A server that could not resume sends the screen whole, and says so.
  reopened.push({ id: "first", data: Buffer.from("redrawn").toString("base64"), at: 900, reset: true });
  deepStrictEqual(drawn.at(-1), { text: "redrawn", reset: true });

  stopSecond();
  stopFirst();
});

test("a shell that ends tells its view once", () => {
  const { release } = captureFetch();
  release();
  FakeEventSource.opened.length = 0;

  let exits = 0;
  const stop = watchTerminal("ending", { onData: () => {}, onExit: () => exits++ });
  FakeEventSource.opened.at(-1).push({ id: "ending", at: 10, exit: true });
  strictEqual(exits, 1);
  stop();
});

test("the last view leaving closes the stream", () => {
  const { release } = captureFetch();
  release();
  FakeEventSource.opened.length = 0;

  const stop = watchTerminal("only", { onData: () => {} });
  const stream = FakeEventSource.opened.at(-1);
  stop();
  ok(stream.closed, "nothing is left watching, so nothing should stay open");
});

test("the first size is sent at once and the rest are settled first", async () => {
  const { calls, release } = captureFetch();
  release();

  // Until the shell is told, it is drawing its prompt for a window it is not
  // in, so the first measurement cannot wait.
  resizeTerminal("sized", 120, 40);
  await new Promise((r) => setTimeout(r, 10));
  strictEqual(calls.length, 1, "the first size should not be held back");
  deepStrictEqual(JSON.parse(calls[0].body), { cols: 120, rows: 40 });

  // A drag is a hundred of these, and each one makes a full-screen program
  // redraw, so only where it came to rest is sent.
  resizeTerminal("sized", 121, 40);
  resizeTerminal("sized", 122, 40);
  resizeTerminal("sized", 123, 41);
  strictEqual(calls.length, 1, "a drag should not send a request per step");
  await new Promise((r) => setTimeout(r, 200));
  strictEqual(calls.length, 2);
  deepStrictEqual(JSON.parse(calls[1].body), { cols: 123, rows: 41 });
});
