import test from "node:test";
import assert from "node:assert/strict";
import { setSmartSearch, smartSearch, searchTasks, refreshSmartIndex } from "./smart-search.js";

const flush = () => new Promise((resolve) => setImmediate(resolve));
const workers = [];
globalThis.localStorage = { setItem() {} };
globalThis.fetch = async () => new Response(JSON.stringify([{ id: "task", title: "Test" }]), {
  headers: { "content-type": "application/json" },
});
globalThis.Worker = class {
  messages = [];
  terminated = false;
  constructor() { workers.push(this); }
  postMessage(data) { this.messages.push(data); }
  terminate() { this.terminated = true; }
  reply(data) { this.onmessage({ data }); }
};

test("disabling cancels indexing and ignores late progress from the old worker", async () => {
  try {
    setSmartSearch(true);
    await flush();
    const old = workers.at(-1);
    assert.equal(old.messages[0].type, "index");
    old.reply({ progress: { phase: "download", percent: 35 } });
    assert.equal(smartSearch.percent, 35);
    setSmartSearch(false);
    await flush();
    assert.equal(old.terminated, true);
    assert.equal(smartSearch.phase, "idle");
    setSmartSearch(true);
    await flush();
    old.reply({ progress: { phase: "ready", percent: 100 } });
    assert.equal(smartSearch.phase, "loading");
  } finally { setSmartSearch(false); await flush(); }
});

test("queries wait for indexing, errors are surfaced and a retry creates a new worker", async () => {
  try {
    setSmartSearch(true);
    await flush();
    const worker = workers.at(-1);
    const result = searchTasks("query", ["task"]);
    assert.equal(worker.messages.length, 1);
    worker.reply({ progress: { phase: "ready", total: 1 } });
    worker.reply({ id: worker.messages[0].id });
    await flush();
    assert.equal(worker.messages[1].type, "search");
    worker.reply({ id: worker.messages[1].id, result: { task: 0.8 } });
    assert.deepEqual(await result, { task: 0.8 });
    const indexing = refreshSmartIndex();
    await flush();
    worker.reply({ id: worker.messages.at(-1).id, error: "Disk full" });
    await indexing;
    assert.equal(smartSearch.phase, "error");
    assert.equal(smartSearch.error, "Disk full");
    setSmartSearch(true);
    await flush();
    assert.notEqual(workers.at(-1), worker);
    assert.equal(smartSearch.error, "");
  } finally { setSmartSearch(false); await flush(); }
});
