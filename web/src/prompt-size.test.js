import { strictEqual } from "node:assert/strict";
import { test } from "node:test";
import { PROMPT_LIMIT, PROMPT_WARN, overPromptLimit, promptLimitNotice, promptSize, promptSizeTone } from "./prompt-size.js";
import { nf } from "./api.js";

// Counts are formatted in the machine's own locale, so the expectations are too.
const n = (value) => nf.format(value);

test("a prompt's size counts characters exactly and tokens as an estimate", () => {
  for (const [text, expected] of [
    ["", ""],
    [null, ""],
    [undefined, ""],
    ["x", "1 character · ~1 token"],
    ["xx", "2 characters · ~1 token"],
    ["x".repeat(400), "400 characters · ~100 tokens"],
    ["x".repeat(6000), `${n(6000)} characters · ~1.5k tokens`],
    ["x".repeat(PROMPT_LIMIT + 1), `${n(32001)} characters · ~8.0k tokens · over the ${n(PROMPT_LIMIT)} limit, not saved`],
  ]) strictEqual(promptSize(text), expected);
});

// Text over the limit that is already stored — written before the limit — is
// over it but not unsaved.
test("a stored prompt over the limit is not called unsaved", () => {
  const text = "x".repeat(PROMPT_LIMIT + 1);
  strictEqual(promptSize(text, true), `${n(32001)} characters · ~8.0k tokens · over the ${n(PROMPT_LIMIT)} limit`);
  strictEqual(promptSize("x".repeat(10), true), `${n(10)} characters · ~3 tokens`);
});

test("the size turns amber past the warning and red past the limit", () => {
  for (const [text, expected] of [
    ["", "text-dimmed"],
    ["x".repeat(PROMPT_WARN), "text-dimmed"],
    ["x".repeat(PROMPT_WARN + 1), "text-warning"],
    ["x".repeat(PROMPT_LIMIT), "text-warning"],
    ["x".repeat(PROMPT_LIMIT + 1), "text-error"],
  ]) strictEqual(promptSizeTone(text), expected);
  strictEqual(overPromptLimit("x".repeat(PROMPT_LIMIT)), false);
  strictEqual(overPromptLimit("x".repeat(PROMPT_LIMIT + 1)), true);
  strictEqual(
    promptLimitNotice("x".repeat(PROMPT_LIMIT + 1)),
    `${n(32001)} characters; the limit is ${n(PROMPT_LIMIT)}. Shorten it and click away to save.`,
  );
});
