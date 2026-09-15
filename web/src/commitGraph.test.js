import { test } from "node:test";
import assert from "node:assert/strict";
import { commitGraph } from "./commitGraph.js";

const commit = (hash, ...parents) => ({ hash, parents });

test("linear history stays in one lane and stops at the root", () => {
  const graph = commitGraph([commit("a", "b"), commit("b", "c"), commit("c")]);
  assert.equal(graph.width, 20);
  assert.deepEqual([...graph.rows.values()].map((r) => r.column), [0, 0, 0]);
  assert.deepEqual(graph.rows.get("a").edges.map((e) => e.kind), ["out"]);
  assert.deepEqual(graph.rows.get("c").edges.map((e) => e.kind), ["in"]);
});

test("merge splits into colored lanes which converge at their common ancestor", () => {
  const graph = commitGraph([commit("merge", "main", "feature"), commit("feature", "base"), commit("main", "base"), commit("base")]);
  const split = graph.rows.get("merge").edges;
  assert.deepEqual(split.map((e) => e.to), [0, 1]);
  assert.notEqual(split[0].color, split[1].color);
  assert.equal(graph.rows.get("feature").column, 1);
  assert.equal(graph.rows.get("base").column, 0);
  assert.equal(graph.rows.get("base").edges.length, 1);
});

test("filtering follows hidden ancestors and deduplicates converging paths", () => {
  const graph = commitGraph([commit("a", "b", "c"), commit("b", "d"), commit("c", "d"), commit("d")], new Set(["a", "d"]));
  assert.equal(graph.rows.size, 2);
  assert.equal(graph.rows.get("a").edges.length, 1);
  assert.equal(graph.rows.get("d").edges[0].kind, "in");
});

test("disconnected roots have no invented edges and limited history keeps continuation", () => {
  const graph = commitGraph([commit("a"), commit("b", "outside")]);
  assert.equal(graph.rows.get("a").edges.length, 0);
  assert.deepEqual(graph.rows.get("b").edges.map((e) => e.kind), ["out"]);
});

test("octopus merges keep all parents", () => {
  const graph = commitGraph([commit("a", "b", "c", "d"), commit("b"), commit("c"), commit("d")]);
  assert.deepEqual(graph.rows.get("a").edges.map((e) => e.to), [0, 1, 2]);
  assert.equal(graph.width, 44);
});
