import { fuzzyAny } from "./fuzzy.js";

export const TASK_TIME_FILTERS = [
  { label: "Last 24h", value: "1d" },
  { label: "Last week", value: "7d" },
  { label: "Last month", value: "1mo" },
  { label: "All times", value: "all" },
];
const days = { "1d": 1, "7d": 7, "1mo": 30 };

// Also used by the project watcher to request an index refresh after edits.
export function taskText(task) {
  return [task.title || "Untitled task", task.prompt, task.summary].filter(Boolean).join("\n");
}

export function filterTaskRows(tasks, query, window, scores = null, now = Date.now()) {
  const since = days[window] ? now - days[window] * 86400000 : 0;
  const q = query.trim();
  const rows = tasks.filter((task) => !since || task.last_active_at >= since)
    .filter((task) => !q || (scores
      ? Number.isFinite(scores[task.id]) && scores[task.id] >= 0.3
      : fuzzyAny([task.title || "Untitled task"], q) !== null));
  if (q && scores) rows.sort((a, b) => scores[b.id] - scores[a.id]);
  return rows.map((task) => ({ task, archived: !!task.done_at }));
}
