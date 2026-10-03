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
