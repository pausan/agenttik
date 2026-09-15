import { strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { isoDate, usageBreakdownRows } from "./api.js";

test("stats dates use a locale-independent ISO timestamp", () => {
  strictEqual(isoDate(Date.UTC(2026, 8, 12, 13, 32, 11)), "2026-09-12 13:32:11");
  strictEqual(isoDate(0), "never");
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
