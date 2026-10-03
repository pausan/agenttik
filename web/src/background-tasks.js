/* Background tasks: shells and subagents a running turn has left working
   behind it. The server sends each one's whole state whenever it changes, so
   the latest copy replaces the one before. See specs/083-background-tasks.md. */

/* upsertTask hands back the list with task in it, replacing any copy with
   the same id, oldest first. */
export function upsertTask(list, task) {
  const next = (list || []).filter((t) => t.id !== task.id);
  next.push(task);
  return next.sort((a, b) => a.started_at - b.started_at);
}

/* taskSeconds is how long a task has run: until it ended, or until now. */
export function taskSeconds(task, now) {
  return Math.max(0, Math.floor(((task.ended_at || now) - task.started_at) / 1000));
}

/* durationLabel reads like a stopwatch once past a minute: 42s, 1:05, 1:02:03. */
export function durationLabel(secs) {
  if (secs < 60) return `${secs}s`;
  const pad = (n) => String(n).padStart(2, "0");
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return h ? `${h}:${pad(m)}:${pad(secs % 60)}` : `${m}:${pad(secs % 60)}`;
}

/* tasksLabel names the list: how many are still running, and how many there
   are once none is. */
export function tasksLabel(tasks) {
  const running = tasks.filter((t) => t.status === "running").length;
  const n = running || tasks.length;
  const noun = n === 1 ? "background task" : "background tasks";
  return running ? `${n} ${noun} running` : `${n} ${noun} finished`;
}
