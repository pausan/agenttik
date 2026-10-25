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
