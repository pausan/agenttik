import test from "node:test";
import assert from "node:assert/strict";
import { setSmartSearch, smartSearch, searchTasks, refreshSmartIndex } from "./smart-search.js";

const flush = () => new Promise((resolve) => setImmediate(resolve));
const response = (data, status = 200) => new Response(JSON.stringify(data), {
  status, headers: { "content-type": "application/json" },
});
const ready = { phase: "ready", percent: 100, total: 1, version: 1 };
globalThis.localStorage = { setItem() {} };

test("disabling ignores late backend progress and preserves a later enable", async () => {
  const pending = [];
  globalThis.fetch = () => new Promise((resolve) => pending.push(resolve));
  try {
    setSmartSearch(true);
    setSmartSearch(false);
    setSmartSearch(true);
    pending[0](response(ready));
    await flush();
    assert.equal(smartSearch.phase, "loading");
    pending[1](response({ phase: "download", percent: 35 }));
    await flush();
    assert.equal(smartSearch.percent, 35);
    setSmartSearch(false);
    await flush();
    assert.equal(smartSearch.phase, "idle");
  } finally { setSmartSearch(false); }
});

test("queries wait for indexing and send only the query and task ids", async () => {
  let finish;
  const calls = [];
  globalThis.fetch = async (path, options) => {
    calls.push([path, options]);
    if (path.endsWith("/index")) return new Promise((resolve) => { finish = resolve; });
    return response({ task: 0.8 });
  };
  try {
    setSmartSearch(true);
    const result = searchTasks("query", ["task"]);
    assert.equal(calls.length, 1);
    finish(response(ready));
    assert.deepEqual(await result, { task: 0.8 });
    assert.equal(calls[1][0], "/api/smart-search/query");
    assert.deepEqual(JSON.parse(calls[1][1].body), { query: "query", ids: ["task"] });
  } finally { setSmartSearch(false); }
});

test("backend errors are surfaced and retry recovers", async () => {
  globalThis.fetch = async () => response({ error: "Disk full" }, 503);
  try {
    setSmartSearch(true);
    await flush();
    assert.equal(smartSearch.phase, "error");
    assert.equal(smartSearch.error, "Disk full");
    globalThis.fetch = async () => response(ready);
    setSmartSearch(true);
    await flush();
    assert.equal(smartSearch.phase, "ready");
    assert.equal(smartSearch.error, "");
    await refreshSmartIndex();
    assert.equal(smartSearch.version, 1);
  } finally { setSmartSearch(false); }
});

test("in-flight query results cannot survive disabling", async () => {
  let finish;
  globalThis.fetch = async (path) => path.endsWith("/index") ? response(ready)
    : new Promise((resolve) => { finish = resolve; });
  try {
    setSmartSearch(true);
    await flush();
    const result = searchTasks("query", ["task"]);
    setSmartSearch(false);
    finish(response({ task: 0.8 }));
    await assert.rejects(result, /stopped/);
    assert.equal(smartSearch.phase, "idle");
  } finally { setSmartSearch(false); }
});
