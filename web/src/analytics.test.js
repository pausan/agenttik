import test from "node:test";
import assert from "node:assert/strict";
import { analyticsGroups, analyticsShare, analyticsTotals } from "./analytics.js";

const rows = [
  { provider: "claude", account_id: 1, project_id: 1, project_name: "Alpha", session_id: "a", cost_usd: 2, input_tokens: 100, turns: 1, cost_turns: 1 },
  { provider: "claude", account_id: 2, project_id: 1, project_name: "Alpha", session_id: "a", cost_usd: 4, input_tokens: 200, turns: 2, cost_turns: 2 },
  { provider: "claude", account_id: 1, project_id: 2, project_name: "Beta", session_id: "b", cost_usd: 5, input_tokens: 50, turns: 1, cost_turns: 1 },
  { provider: "codex", account_id: 0, project_id: 2, project_name: "Beta", session_id: "b", cost_usd: 0, input_tokens: 900, turns: 3, inferred_turns: 1 },
];

test("provider totals combine subscriptions and rank projects by their share", () => {
  const groups = analyticsGroups(rows);
  assert.equal(groups[0].provider, "claude");
  assert.equal(groups[0].cost_usd, 11);
  assert.equal(groups[0].projects[0].name, "Alpha");
  assert.equal(groups[0].projects[0].cost_usd, 6);
  assert.equal(groups[0].projects[0].rows[0].account_id, 2);
  assert.equal(analyticsShare(6, 11), "54.5%");
  assert.equal(analyticsShare(0, 0), "—");
  assert.equal(analyticsTotals(rows).turns, 7);
  assert.equal(analyticsTotals(rows).inferred_turns, 1);
});

test("subscription groups keep separate denominators and token ranking works without cost", () => {
  const groups = analyticsGroups(rows, "subscription", "input_tokens");
  assert.equal(groups.length, 3);
  assert.equal(groups[0].provider, "codex");
  assert.equal(groups[0].cost_turns, 0);
  const personal = groups.find((group) => group.provider === "claude" && group.account_id === 1);
  assert.equal(personal.input_tokens, 150);
  assert.equal(personal.projects[0].id, 1);
  assert.equal(analyticsShare(personal.projects[0].input_tokens, personal.input_tokens), "66.7%");
  assert.equal(rows[0].cost_usd, 2);
  assert.deepEqual(analyticsGroups([]), []);
});

test("model grouping combines effort and task rows while chart ranks only used combinations", () => {
  const usage = [
    { ...rows[0], model: "a", effort: "high" },
    { ...rows[0], model: "a", effort: "low", cost_usd: 3 },
    { ...rows[0], model: "b", effort: "high", cost_usd: 0, input_tokens: 500 },
  ];
  const models = analyticsGroups(usage, "model");
  assert.equal(models.length, 2);
  assert.equal(models[0].model, "a");
  assert.equal(models[0].cost_usd, 5);
  assert.equal(models[0].projects[0].rows.length, 1);
  assert.equal(models[0].projects[0].rows[0].turns, 2);
  const chart = analyticsGroups(usage, "model_effort");
  assert.deepEqual(chart.map((g) => [g.model, g.effort, g.cost_usd]), [["a", "low", 3], ["a", "high", 2], ["b", "high", 0]]);
  assert.equal(analyticsGroups(usage, "model_effort", "input_tokens")[0].model, "b");
  assert.equal(analyticsTotals(chart).cost_usd, analyticsTotals(usage).cost_usd);
  assert.deepEqual(analyticsGroups([], "model_effort"), []);
});
