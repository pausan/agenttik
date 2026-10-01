import test from "node:test";
import assert from "node:assert/strict";
import { answersFor, emptyAnswers, pick, type } from "./questions.js";

const color = { id: "color", question: "Color?", options: [{ label: "Red" }, { label: "Blue" }], other: true };
const sizes = { id: "sizes", question: "Sizes?", options: [{ label: "S" }, { label: "M" }], multi: true, other: true };
const strict = { id: "go", question: "Go?", options: [{ label: "Yes" }, { label: "No" }] };

test("every question needs an answer before the form can be sent", () => {
  const state = emptyAnswers([color, sizes]);
  assert.equal(answersFor([color, sizes], state), null);
  pick(state, color, "Red");
  assert.equal(answersFor([color, sizes], state), null);
  pick(state, sizes, "S");
  assert.deepEqual(answersFor([color, sizes], state), { color: ["Red"], sizes: ["S"] });
});

test("a single choice keeps the last pick or the typed text, never both", () => {
  const state = emptyAnswers([color]);
  pick(state, color, "Red");
  pick(state, color, "Blue");
  assert.deepEqual(answersFor([color], state), { color: ["Blue"] });
  type(state, color, "Teal");
  assert.deepEqual(state.color.picked, []);
  assert.deepEqual(answersFor([color], state), { color: ["Teal"] });
  pick(state, color, "Red");
  assert.equal(state.color.typed, "");
  pick(state, color, "Red");
  assert.equal(answersFor([color], state), null, "picking again unpicks");
});

test("a multiple choice adds the typed text to the picks", () => {
  const state = emptyAnswers([sizes]);
  pick(state, sizes, "S");
  pick(state, sizes, "M");
  pick(state, sizes, "S");
  type(state, sizes, " XL ");
  assert.deepEqual(answersFor([sizes], state), { sizes: ["M", "XL"] });
});

test("typed text does not count where typing is not offered, nor when blank", () => {
  const state = emptyAnswers([strict, color]);
  type(state, strict, "maybe");
  type(state, color, "   ");
  assert.equal(answersFor([strict, color], state), null);
});
