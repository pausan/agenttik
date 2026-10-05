/* Background tasks keep one transcript row each, updated in place with the
   latest state from the server. See specs/083-background-tasks.md. */

export function upsertBackgroundMessage(messages, message) {
  const at = messages.findIndex((m) => m.id === message.id);
  if (at >= 0) {
    messages[at] = message;
    return;
  }
  // Only a new task interrupts prose. Updating an earlier task must leave
  // the current assistant message streaming.
  const last = messages.at(-1);
  if (last?.streaming) last.streaming = false;
  messages.push(message);
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
