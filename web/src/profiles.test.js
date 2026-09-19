import { test } from "node:test";
import { strictEqual, deepEqual } from "node:assert/strict";

test("profile URLs keep requests, streams and files in the selected profile", async () => {
  globalThis.location = { search: "?profile=work" };
  const { apiURL, instanceKey } = await import("./api.js?profile-routing");
  strictEqual(apiURL("/api/projects"), "/api/projects?profile=work");
  strictEqual(apiURL("/api/stream?session=s1"), "/api/stream?session=s1&profile=work");
  strictEqual(apiURL("/api/attachments/image.png"), "/api/attachments/image.png?profile=work");
  strictEqual(apiURL("https://example.com"), "https://example.com");
  strictEqual(instanceKey("tabs"), "tabs:work");
  delete globalThis.location;
});

test("private state never reads or writes persistent browser storage", async () => {
  const { storage, setPrivateMode } = await import("./api.js?private-storage");
  const calls = [];
  globalThis.localStorage = {
    getItem(key) { calls.push(key); return "normal"; },
    setItem(key, value) { calls.push([key, value]); },
    removeItem(key) { calls.push(key); },
  };
  setPrivateMode(true);
  strictEqual(storage.getItem("draft"), null);
  storage.setItem("draft", "private prompt");
  strictEqual(storage.getItem("draft"), "private prompt");
  storage.removeItem("draft");
  strictEqual(storage.getItem("draft"), null);
  deepEqual(calls, []);
  delete globalThis.localStorage;
});

test("profile shortcuts follow saved order, wrap, and preserve URL state", async () => {
  globalThis.location = { search: "" };
  let assigned;
  globalThis.window = { location: { href: "http://localhost/?remote=x#task", assign(url) { assigned = url; } } };
  const { profiles, switchAdjacentProfile } = await import("./profiles.js");
  profiles.items = [{ id: "work" }, { id: "default" }, { id: "personal" }];
  switchAdjacentProfile(1);
  strictEqual(assigned, "http://localhost/?remote=x&profile=personal#task");
  switchAdjacentProfile(-1);
  strictEqual(assigned, "http://localhost/?remote=x&profile=work#task");
  profiles.items = [{ id: "default" }, { id: "work" }, { id: "personal" }];
  switchAdjacentProfile(-1);
  strictEqual(assigned, "http://localhost/?remote=x&profile=personal#task");
  assigned = undefined;
  profiles.items = [{ id: "default" }];
  switchAdjacentProfile(1);
  strictEqual(assigned, undefined);
  delete globalThis.location;
  delete globalThis.window;
});
