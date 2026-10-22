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

/* runningTaskCount is how many of the transcript's tasks are still running. */
export function runningTaskCount(messages) {
  let count = 0;
  for (const m of messages) {
    if (m.role !== "background") continue;
    try {
      if (JSON.parse(m.content).status === "running") count++;
    } catch {
      /* an unreadable row is not a running task */
    }
  }
  return count;
}

/* takeFollowUp places a prompt the running turn has just started on, as the
   server stored it. The bubble drawn on send has no id yet, and it can sit
   before the end of a reply that was still streaming, so it moves to the end.
   See specs/083-background-tasks.md. */
export function takeFollowUp(messages, message) {
  if (messages.some((m) => m.id === message.id)) return;
  const at = messages.findLastIndex((m) => !m.id && m.role === "user" && m.content === message.content);
  if (at >= 0) messages.splice(at, 1);
  const last = messages.at(-1);
  if (last?.streaming) last.streaming = false;
  messages.push(message);
}

/* wakesAgent says an event shows the main agent at work again, between
   replies of a turn that waits on background tasks. A background subagent's
   tool calls do not: it works while the main agent waits. */
export function wakesAgent(ev) {
  if (ev.type === "tool_use") return !ev.tool?.subagent;
  return ["text", "thinking", "follow_up"].includes(ev.type);
}

/* waitingLabel is the progress line of a turn that takes prompts. */
export function waitingLabel(waiting, tasks) {
  const count = tasks === 1 ? "1 background task" : tasks ? `${tasks} background tasks` : "background tasks";
  return waiting ? `Waiting for ${count} · you can keep writing` : `Working · ${count} · you can keep writing`;
}
