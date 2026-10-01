import { watch } from "vue";

// One timer for the visible transcript, independent of how many rows show it.
export function trackTaskAttention(unread, currentTask, visible, save = () => {}) {
  // A manual mark on the current transcript lasts until the next visit.
  let held = null;
  const stop = watch(
    () => {
      const id = currentTask();
      return [id, id ? unread[id] : null, visible()];
    },
    ([id, turn, shown], previous, cleanup) => {
      if (id !== previous?.[0] || turn !== -1) held = null;
      if (turn === -1 && previous?.[0] === id && previous?.[1] !== -1) held = id;
      if (held === id) return;
      if (!id || !turn || !shown) return;
      const timer = setTimeout(() => {
        delete unread[id];
        save();
      }, 3000);
      cleanup(() => clearTimeout(timer));
    },
    { immediate: true, flush: "sync" },
  );
  return stop;
}

export function taskDot(status, unread) {
  return unread && status !== "error" && status !== "running" ? "unread" : status;
}

// A task waiting on a tool approval needs the user as much as an unread one,
// and a folded project shows no task rows to say so.
export function projectDot(project, unread, approvals = {}) {
  const running = project.recent_sessions.some((task) => task.status === "running") ||
    project.schedules.some((schedule) => schedule.running);
  const attention = project.recent_sessions.some((task) =>
    approvals[task.id] || taskDot(task.status, unread[task.id]) === "unread");
  return attention ? (running ? "unread-running" : "unread") : (running ? "running" : null);
}
