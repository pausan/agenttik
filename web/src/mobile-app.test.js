import test from "node:test";
import assert from "node:assert/strict";
import { mobileApp } from "./mobile-app.js";

test("browsers have no mobile app channel", () => {
  assert.equal(mobileApp({}), null);
  assert.equal(mobileApp({ agenttikApp: {} }), null);
});

for (const [name, scope] of [
  ["Android", (channel) => ({ agenttikApp: channel })],
  ["iOS", (channel) => ({ webkit: { messageHandlers: { agenttikApp: channel } } })],
]) {
  test(`${name} logout signs out before disconnecting`, async () => {
    const calls = [];
    const app = mobileApp(scope({ postMessage: (message) => calls.push(message) }));
    await app.logout(async (url, options) => { calls.push(url); assert.equal(options.redirect, "manual"); });
    assert.deepEqual(calls, ["/__auth/logout", "disconnect"]);
  });
}

test("logout still disconnects when the server is gone", async () => {
  const calls = [];
  await mobileApp({ agenttikApp: { postMessage: (message) => calls.push(message) } }).logout(async () => { throw new Error("offline"); });
  assert.deepEqual(calls, ["disconnect"]);
});

function refreshed(fetcher) {
  const calls = [];
  return mobileApp({ agenttikApp: { postMessage: (message) => calls.push(message) } })
    .refresh(fetcher, 0).then(() => calls);
}

test("refresh reports where the server can be reached", async () => {
  const addresses = ["http://192.168.1.5:7717", "https://lab.trycloudflare.com"];
  const calls = await refreshed(async () => ({ ok: true, json: async () => ({ addresses }) }));
  assert.deepEqual(calls, ["addresses:" + JSON.stringify(addresses)]);
});

test("refresh says nothing when the server answers without addresses", async () => {
  assert.deepEqual(await refreshed(async () => ({ ok: false })), []);
  assert.deepEqual(await refreshed(async () => ({ ok: true, json: async () => { throw new Error("html"); } })), []);
});

test("refresh gives one dropped request a second chance", async () => {
  let tries = 0;
  const calls = await refreshed(async () => {
    if (++tries === 1) throw new Error("offline");
    return { ok: false };
  });
  assert.equal(tries, 2);
  assert.deepEqual(calls, []);
});

test("refresh hands over when the server stays unreachable", async () => {
  assert.deepEqual(await refreshed(async () => { throw new Error("offline"); }), ["lost"]);
});

test("watch checks when the phone comes back to the page, not while hidden", async (t) => {
  t.mock.timers.enable({ apis: ["setInterval"] });
  const requests = [];
  let changed;
  const scope = {
    agenttikApp: { postMessage() {} },
    fetch: async (url) => { requests.push(url); return { ok: false }; },
    document: { visibilityState: "hidden", addEventListener: (_, fn) => { changed = fn; } },
  };
  mobileApp(scope).watch();
  t.mock.timers.tick(5 * 60 * 1000);
  assert.deepEqual(requests, []);
  scope.document.visibilityState = "visible";
  changed();
  t.mock.timers.tick(5 * 60 * 1000);
  assert.deepEqual(requests, ["/api/server/addresses", "/api/server/addresses"]);
});
