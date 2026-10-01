/* Answers to an agent's questions, as the card collects them. Each question
   holds the options picked and any text typed. A single-choice question takes
   one or the other — typing replaces the pick — and a multi-choice one takes
   both. See specs/082-tool-approvals.md. */

export function emptyAnswers(questions) {
  return Object.fromEntries(questions.map((q) => [q.id, { picked: [], typed: "" }]));
}

/* pick toggles an option. Picking in a single-choice question replaces the
   pick and clears the typed text, so the answer is what was last chosen. */
export function pick(state, question, label) {
  const entry = state[question.id];
  if (question.multi) {
    entry.picked = entry.picked.includes(label) ? entry.picked.filter((l) => l !== label) : [...entry.picked, label];
    return;
  }
  entry.picked = entry.picked[0] === label ? [] : [label];
  entry.typed = "";
}

export function type(state, question, text) {
  const entry = state[question.id];
  entry.typed = text;
  if (!question.multi && text.trim()) entry.picked = [];
}

/* answersFor is what goes to the server, or null while a question is still
   unanswered. */
export function answersFor(questions, state) {
  const answers = {};
  for (const q of questions) {
    const entry = state[q.id] || { picked: [], typed: "" };
    const typed = q.other ? entry.typed.trim() : "";
    const got = q.multi ? [...entry.picked, ...(typed ? [typed] : [])] : typed ? [typed] : entry.picked.slice(0, 1);
    if (!got.length) return null;
    answers[q.id] = got;
  }
  return answers;
}
