import { nf, tokens } from "./api.js";

/* What a standing prompt costs, shown under the field it is written in: a
   mode's text and a project's prompt open every conversation they are linked
   to, so their length is the reader's to judge. Characters are exact; tokens
   are an estimate at four characters each, the usual rule for prose — enough
   to tell a paragraph from a pasted document, and never presented as the
   provider's own count. See specs/101-modes.md. */
/* Past PROMPT_WARN the size is shown in amber: a standing prompt that long is
   usually a document someone pasted rather than a way of working — the
   orchestrator's own prompt, the longest the app ships, is under 7,000. Past
   PROMPT_LIMIT the field stops saving and the server refuses too. */
export const PROMPT_WARN = 8_000;
export const PROMPT_LIMIT = 32_000;

export function overPromptLimit(text) {
  return (text || "").length > PROMPT_LIMIT;
}

/* Plain, amber over the warning, red over the limit. */
export function promptSizeTone(text) {
  const chars = (text || "").length;
  if (chars > PROMPT_LIMIT) return "text-error";
  return chars > PROMPT_WARN ? "text-warning" : "text-dimmed";
}

/* Said in a toast when a field refuses to save: over the limit the size line
   under the field can be below the fold, and blur is when it matters. */
export function promptLimitNotice(text) {
  return `${nf.format((text || "").length)} characters; the limit is ${nf.format(PROMPT_LIMIT)}. Shorten it and click away to save.`;
}

/* saved says the field holds the text already stored, which an over-limit
   prompt written before the limit does: it stays as it is and still opens its
   conversations, so it is not called unsaved. */
export function promptSize(text, saved = false) {
  const chars = (text || "").length;
  if (!chars) return "";
  const estimate = Math.max(1, Math.round(chars / 4));
  const unit = estimate === 1 ? "token" : "tokens";
  const size = `${nf.format(chars)} ${chars === 1 ? "character" : "characters"} · ~${tokens(estimate)} ${unit}`;
  if (chars <= PROMPT_LIMIT) return size;
  return `${size} · over the ${nf.format(PROMPT_LIMIT)} limit${saved ? "" : ", not saved"}`;
}
