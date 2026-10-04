import { strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { isoDate, taskStats, turnSummary, usageBreakdownRows, usageCost } from "./api.js";

test("stats dates use a locale-independent ISO timestamp", () => {
  strictEqual(isoDate(Date.UTC(2026, 8, 12, 13, 32, 11)), "2026-09-12 13:32:11");
  strictEqual(isoDate(0), "never");
});

test("stats mark usage cost when an estimate contributes", () => {
  strictEqual(usageCost({ cost_usd: 2, estimated_cost_usd: 3 }), "≈$5.0000");
  strictEqual(usageCost({ cost_usd: 2, estimated_cost_usd: 0 }), "$2.0000");
});

test("usage rows keep main, subagent, and unscoped tokens distinct", () => {
  const rows = usageBreakdownRows({
    usage_breakdown_turns: 1,
    input_tokens: 115,
    output_tokens: 25,
    main_input_tokens: 60,
    main_output_tokens: 12,
    subagent_input_tokens: 40,
    subagent_output_tokens: 10,
    subagent_count: 2,
  });
  strictEqual(rows[0][0], "Main agent");
  strictEqual(rows[0][1], "60 in · 12 out");
  strictEqual(rows[1][0], "Subagents (2)");
  strictEqual(rows[1][1], "40 in · 10 out");
  strictEqual(rows[2][0], "Unattributed");
  strictEqual(rows[2][1], "15 in · 3 out");
});

test("usage rows stay hidden when a provider cannot attribute agents", () => {
  strictEqual(usageBreakdownRows({ input_tokens: 10 }).length, 0);
});

test("live Codex usage adds to finished turns until final stats replace it", () => {
  const detail = {
    running: true, turns: [{ id: 1 }, { id: 2 }],
    stats: { turns: 1, input_tokens: 100, output_tokens: 20, main_input_tokens: 100,
      usage_breakdown_turns: 1 },
    liveUsage: { input_tokens: 30, output_tokens: 5, main_input_tokens: 20,
      subagent_input_tokens: 10, subagent_count: 1 },
  };
  const live = taskStats(detail);
  strictEqual(live.turns, 2);
  strictEqual(live.input_tokens, 130);
  strictEqual(live.main_input_tokens, 120);
  strictEqual(live.subagent_input_tokens, 10);
  strictEqual(live.usage_breakdown_turns, 2);
  strictEqual(detail.stats.input_tokens, 100);
  detail.running = false;
  detail.stats = { turns: 2, input_tokens: 130 };
  strictEqual(taskStats(detail), detail.stats);
});

test("remote tabs and drafts are scoped to their server across connection checks", async () => {
  const { api, instanceKey } = await import("./api.js");
  const original = globalThis.fetch;
  try {
    strictEqual(instanceKey("agenttik.openTabs"), "agenttik.openTabs");
    globalThis.fetch = async () => new Response("{}", { headers: { "Content-Type": "application/json", "X-Agenttik-Remote": "https://one.example" } });
    await api("GET", "/api/projects");
    strictEqual(instanceKey("agenttik.openTabs"), "agenttik.openTabs:https://one.example");
    globalThis.fetch = async () => new Response("{}", { headers: { "Content-Type": "application/json" } });
    await api("POST", "/api/remote/connect", { address: "two.example" });
    strictEqual(instanceKey("agenttik.openTabs"), "agenttik.openTabs:https://one.example");
    globalThis.fetch = async () => new Response("{}", { headers: { "Content-Type": "application/json", "X-Agenttik-Remote": "https://two.example" } });
    await api("GET", "/api/projects");
    strictEqual(instanceKey("agenttik.openTabs"), "agenttik.openTabs:https://two.example");
  } finally {
    globalThis.fetch = original;
  }
});

test("diagnostics never write browser storage before privacy is known or in private mode", async () => {
  const { diagnosticStorage, setPrivateMode } = await import("./api.js?diagnostic-privacy");
  const original = globalThis.localStorage;
  let writes = 0;
  globalThis.localStorage = { setItem() { writes++; }, getItem() { return null; } };
  try {
    diagnosticStorage.setItem("errors", "startup");
    strictEqual(diagnosticStorage.getItem("errors"), "startup");
    strictEqual(writes, 0);
    setPrivateMode(true);
    diagnosticStorage.setItem("errors", "private");
    strictEqual(diagnosticStorage.getItem("errors"), "private");
    strictEqual(writes, 0);
    setPrivateMode(false);
    diagnosticStorage.setItem("errors", "normal");
    strictEqual(writes, 1);
  } finally { globalThis.localStorage = original; }
});

test("client preferences share storage across profiles and remote instances", async () => {
  const originalLocation = globalThis.location;
  const originalStorage = globalThis.localStorage;
  const originalFetch = globalThis.fetch;
  const values = new Map();
  globalThis.localStorage = {
    getItem(key) { return values.get(key) ?? null; },
    setItem(key, value) { values.set(key, String(value)); },
  };
  try {
    globalThis.location = { search: "" };
    const local = await import("./api.js?client-local");
    globalThis.location = { search: "?profile=work&remote=office" };
    const remote = await import("./api.js?client-remote");
    globalThis.fetch = async () => new Response("{}", { headers: {
      "Content-Type": "application/json", "X-Agenttik-Instance": "local-id", "X-Agenttik-Remote": "office-url",
    } });
    await remote.api("GET", "/api/projects");
    local.clientStorage.setItem("keys", "shared bindings");
    strictEqual(remote.clientStorage.getItem("keys"), "shared bindings");
    remote.clientStorage.setItem("keys", "new bindings");
    strictEqual(local.clientStorage.getItem("keys"), "new bindings");
    // Tabs and drafts still belong to their profile and server.
    remote.storage.setItem("tabs", "remote tabs");
    strictEqual(local.storage.getItem("tabs"), null);
    strictEqual(remote.storage.getItem("tabs"), "remote tabs");
  } finally {
    globalThis.location = originalLocation;
    globalThis.localStorage = originalStorage;
    globalThis.fetch = originalFetch;
  }
});

test("private client preferences never read or change normal bindings", async () => {
  const originalStorage = globalThis.localStorage;
  globalThis.localStorage = {
    getItem() { throw new Error("read normal storage"); },
    setItem() { throw new Error("wrote normal storage"); },
  };
  try {
    const { clientStorage, setPrivateMode } = await import("./api.js?private-client");
    setPrivateMode(true);
    strictEqual(clientStorage.getItem("keys"), null);
    clientStorage.setItem("keys", "private bindings");
    strictEqual(clientStorage.getItem("keys"), "private bindings");
  } finally { globalThis.localStorage = originalStorage; }
});

test("turn footers are exact under two minutes and rough after", () => {
  const turn = (secs, input, output, cost = 0, estimate = 0) =>
    turnSummary([{ started_at: 0, ended_at: secs * 1000, input_tokens: input, output_tokens: output, cost_usd: cost, estimated_cost_usd: estimate }]);
  strictEqual(turn(42, 850, 12, 0.004), "42s · 850 in / 12 out · <$0.01");
  strictEqual(turn(95, 2400, 310, 0.1234), "1min 35s · 2.4k in / 310 out · $0.12");
  strictEqual(turn(3790, 1_312_000, 29_400, 0, 3.219), "1h 3min · 1.3M in / 29k out · ≈$3.22");
  strictEqual(turn(125, 0, 0), "2min · 0 in / 0 out");
  strictEqual(turn(7200, 12_600_000, 1_000), "2h · 13M in / 1k out");
});

test("later turn footers add the task's totals so far", () => {
  const turns = [
    { started_at: 0, ended_at: 100_000, input_tokens: 1_000_000, output_tokens: 20_000, cost_usd: 3, estimated_cost_usd: 0 },
    { started_at: 200_000, ended_at: 230_000, input_tokens: 300_000, output_tokens: 9_000, cost_usd: 0, estimated_cost_usd: 0.22 },
  ];
  strictEqual(turnSummary(turns), "30s · 300k in / 9k out · ≈$0.22 — total 2min · 1.3M in / 29k out · ≈$3.22");
});
