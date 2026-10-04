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
  globalThis.window = { location: { href: "http://localhost/?debug=x#task", assign(url) { assigned = url; } } };
  const { profiles, switchAdjacentProfile } = await import("./profiles.js");
  profiles.items = [{ id: "work" }, { id: "default" }, { id: "personal" }];
  switchAdjacentProfile(1);
  strictEqual(assigned, "http://localhost/?debug=x&profile=personal#task");
  switchAdjacentProfile(-1);
  strictEqual(assigned, "http://localhost/?debug=x&profile=work#task");
  profiles.items = [{ id: "default" }, { id: "work" }, { id: "personal" }];
  switchAdjacentProfile(-1);
  strictEqual(assigned, "http://localhost/?debug=x&profile=personal#task");
  assigned = undefined;
  profiles.items = [{ id: "default" }];
  switchAdjacentProfile(1);
  strictEqual(assigned, undefined);
  delete globalThis.location;
  delete globalThis.window;
});

test("remote profiles carry their machine on every request and in saved state", async () => {
  globalThis.location = { search: "?remote=m1&profile=work" };
  const { apiURL, instanceKey } = await import("./api.js?remote-routing");
  strictEqual(apiURL("/api/projects"), "/api/projects?remote=m1&profile=work");
  strictEqual(apiURL("/api/stream?session=s1"), "/api/stream?session=s1&remote=m1&profile=work");
  strictEqual(instanceKey("tabs"), "tabs:remote-m1:work");
  globalThis.location = { search: "?remote=m1" };
  const remoteDefault = await import("./api.js?remote-default");
  strictEqual(remoteDefault.apiURL("/api/projects"), "/api/projects?remote=m1");
  strictEqual(remoteDefault.instanceKey("tabs"), "tabs:remote-m1");
  delete globalThis.location;
});

test("profile shortcuts continue into remote machines and back", async () => {
  globalThis.location = { search: "" };
  const assigned = [];
  globalThis.window = { location: { href: "http://localhost/?profile=work", assign(url) { assigned.push(url); } } };
  const { profiles, switchAdjacentProfile, allProfiles } = await import("./profiles.js?remote-cycle");
  profiles.items = [{ id: "default", name: "Default" }];
  profiles.remotes = [{ id: "m1", name: "Office", profiles: [{ id: "default", name: "Default" }, { id: "p2", name: "Work" }] }];
  deepEqual(allProfiles().map(p => [p.remote, p.id]), [["", "default"], ["m1", "default"], ["m1", "p2"]]);
  // From the local Default, the previous profile is the last remote one, which
  // is opened only after its machine answers.
  switchAdjacentProfile(-1);
  strictEqual(profiles.connecting.remote, "m1");
  strictEqual(profiles.connecting.profile, "p2");
  deepEqual(assigned, []);
  profiles.connecting = null;
  delete globalThis.location;
  delete globalThis.window;
});

test("profile activity includes remote work and keeps the current local task in its workspace", async () => {
  const { profiles, hasProfileWork } = await import("./profiles.js");
  profiles.items = [{ id: "default", busy: true }, { id: "work", busy: false }];
  profiles.remotes = [];
  strictEqual(hasProfileWork(), false);
  profiles.remotes = [{ id: "office", profiles: [{ id: "default", busy: true }] }];
  strictEqual(hasProfileWork(), true);
  profiles.remotes[0].profiles[0].busy = false;
  strictEqual(hasProfileWork(), false);
  profiles.items[1].busy = true;
  strictEqual(hasProfileWork(), true);
  profiles.items = [];
  profiles.remotes = [];
});
