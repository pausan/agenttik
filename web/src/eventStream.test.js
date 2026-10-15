import { deepStrictEqual, strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { openEventStream } from "./eventStream.js";

class FakeSocket {
  static all = [];
  constructor(url) {
    this.url = url;
    this.closed = false;
    FakeSocket.all.push(this);
  }
  close() {
    this.closed = true;
  }
}

test("a quick tunnel page opens its streams as websockets", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  // A browser that cannot inflate gzip is sent plain text messages.
  const inflate = globalThis.DecompressionStream;
  delete globalThis.DecompressionStream;
  t.after(() => (globalThis.DecompressionStream = inflate));
  globalThis.WebSocket = FakeSocket;
  FakeSocket.all = [];
  const where = { hostname: "a-b-c.trycloudflare.com", href: "https://a-b-c.trycloudflare.com/x" };
  const es = openEventStream("/api/stream?sessions=1", where);
  strictEqual(FakeSocket.all[0].url, "wss://a-b-c.trycloudflare.com/api/stream?sessions=1");

  const seen = [];
  es.onopen = () => seen.push("open");
  es.onmessage = (e) => seen.push(e.data);
  es.onerror = () => seen.push("error");
  FakeSocket.all[0].onopen();
  FakeSocket.all[0].onmessage({ data: '{"a":1}' });
  FakeSocket.all[0].onclose();
  t.mock.timers.tick(3000);
  strictEqual(FakeSocket.all.length, 2, "reconnects after an error");
  FakeSocket.all[1].onopen();
  deepStrictEqual(seen, ["open", '{"a":1}', "error", "open"]);

  es.close();
  strictEqual(FakeSocket.all[1].closed, true);
  FakeSocket.all[1].onclose();
  t.mock.timers.tick(3000);
  strictEqual(FakeSocket.all.length, 2, "a closed stream stays closed");
});

test("anywhere else it is a plain EventSource", () => {
  globalThis.EventSource = class {
    constructor(url) {
      this.url = url;
    }
  };
  strictEqual(openEventStream("/api/stream", { hostname: "localhost", href: "http://localhost:7717/" }).url, "/api/stream");
});

test("gzipped binary messages are read as one SSE stream", async () => {
  globalThis.WebSocket = FakeSocket;
  FakeSocket.all = [];
  const where = { hostname: "a-b-c.trycloudflare.com", href: "https://a-b-c.trycloudflare.com/x" };
  const es = openEventStream("/api/stream/terminals?ids=t1%3A0", where);
  strictEqual(FakeSocket.all[0].url, "wss://a-b-c.trycloudflare.com/api/stream/terminals?ids=t1%3A0&encoding=gzip");

  const seen = [];
  es.onmessage = (e) => seen.push(e.data);
  const { gzipSync, constants } = await import("node:zlib");
  // One gzip stream, flushed per event and cut mid-event, as the bridge
  // passes it on.
  const sse = ': open\n\ndata: {"a":1}\n\n: ping\n\ndata: two\ndata: lines\n\ndata:{"b":2}\n\n';
  const packed = gzipSync(sse, { flush: constants.Z_SYNC_FLUSH });
  const socket = FakeSocket.all[0];
  for (const part of [packed.subarray(0, 7), packed.subarray(7, 20), packed.subarray(20)]) {
    socket.onmessage({ data: part.buffer.slice(part.byteOffset, part.byteOffset + part.length) });
  }
  for (let i = 0; i < 50 && seen.length < 3; i++) await new Promise((r) => setTimeout(r, 5));
  deepStrictEqual(seen, ['{"a":1}', "two\nlines", '{"b":2}']);
  es.close();
});

/* The bridge ends a stream with a complete gzip trailer and then closes. */
async function gzipSocket(sse) {
  globalThis.WebSocket = FakeSocket;
  FakeSocket.all = [];
  const es = openEventStream("/api/stream", { hostname: "a-b-c.trycloudflare.com", href: "https://a-b-c.trycloudflare.com/" });
  const { gzipSync } = await import("node:zlib");
  const packed = gzipSync(sse);
  const socket = FakeSocket.all[0];
  const send = () => socket.onmessage({ data: packed.buffer.slice(packed.byteOffset, packed.byteOffset + packed.length) });
  return { es, socket, send };
}

const settle = () => new Promise((r) => setTimeout(r, 50));

test("every gzipped event arrives before the close is reported", async (t) => {
  const { es, socket, send } = await gzipSocket("data: 1\n\ndata: 2\n\ndata: 3\n\n");
  const seen = [];
  es.onmessage = (e) => seen.push(e.data);
  es.onerror = () => {
    seen.push("error");
    es.close();
  };
  send();
  socket.onclose(); // before the inflater has produced anything
  await settle();
  deepStrictEqual(seen, ["1", "2", "3", "error"]);
});

test("a closed stream hands on nothing more", async () => {
  const { es, send } = await gzipSocket("data: 1\n\n");
  const seen = [];
  es.onmessage = (e) => seen.push(e.data);
  send();
  es.close();
  await settle();
  deepStrictEqual(seen, []);
});
