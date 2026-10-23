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

/* takeFollowUp places a prompt a turn has just started on, as the server
   stored it: one the running turn held, or one sent again unprocessed, which
   starts a turn of its own. It was stored before, and moved below rows stored
   after it under a new id, so the row it replaces goes, and so does the
   bubble drawn on send, which has no id yet. See
   specs/083-background-tasks.md. */
export function takeFollowUp(messages, message, replaces = 0) {
  if (replaces) {
    const old = messages.findIndex((m) => m.id === replaces);
    if (old >= 0) messages.splice(old, 1);
  }
  const at = messages.findIndex((m) => m.id === message.id);
  if (at >= 0) {
    messages[at] = message;
    return;
  }
  const echo = messages.findLastIndex((m) => !m.id && m.role === "user" && m.content === message.content);
  if (echo >= 0) messages.splice(echo, 1);
  const last = messages.at(-1);
  if (last?.streaming) last.streaming = false;
  messages.push(message);
}

/* promptState says whether the agent has seen a prompt of yours: "waiting"
   while the running turn holds it, "unprocessed" when it has no turn and
   none will take it — the task stopped first, or Stop moved it in from the
   queue. Prompts that ran say nothing, and so does a bubble not stored yet,
   unless sending it joined a running turn. So do the prompts a task spawned
   from a thread copies from it: they have no turn either, but they ran in
   the thread, before the task was created at since. */
export function promptState(message, pending = [], since = 0) {
  if (message.role !== "user" || message.turn_id || message.created_at < since) return "";
  if (!message.id) return message.waiting ? "waiting" : "";
  return pending.includes(message.id) ? "waiting" : "unprocessed";
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
