import assert from "node:assert/strict";
import { test } from "node:test";

import { compactModelLabel } from "./model-labels.js";

test("a vendor the connection already names is dropped", () => {
  assert.equal(compactModelLabel("Anthropic", "Anthropic: Claude Sonnet 4.5"), "Claude Sonnet 4.5");
  assert.equal(compactModelLabel("Anthropic API", "anthropic/claude-sonnet-4.5"), "claude-sonnet-4.5");
  assert.equal(compactModelLabel("Z.ai", "z-ai: GLM 4.6"), "GLM 4.6");
});

test("a vendor the connection does not name is kept", () => {
  assert.equal(compactModelLabel("OpenRouter", "Anthropic: Claude Sonnet 4.5"), "Anthropic: Claude Sonnet 4.5");
  assert.equal(compactModelLabel("OpenRouter", "openai/gpt-5"), "openai/gpt-5");
});

test("a plain model name is left alone", () => {
  assert.equal(compactModelLabel("Codex", "Opus 5"), "Opus 5");
  assert.equal(compactModelLabel("Fake", "Fake Quick"), "Fake Quick");
  assert.equal(compactModelLabel("", ""), "");
});
