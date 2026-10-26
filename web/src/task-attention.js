import { watch } from "vue";

// Remember the latest completion separately from unread, so acknowledging it
// does not make a later snapshot mark the same result unread again.
export function reconcileTaskCompletions(unread, completed, tasks) {
  let changed = false;
  for (const task of tasks) {
    const turn = task.last_completed_turn_id;
    // Older remote servers do not report completion IDs.
    if (!Number.isSafeInteger(turn) || turn < 0) continue;
    const previous = completed[task.id];
    if (previous !== undefined && turn <= previous) continue;
    completed[task.id] = turn;
    if (previous !== undefined && task.status !== "running") unread[task.id] = turn;
    changed = true;
  }
  return changed;
}

// One timer for the visible transcript, independent of how many rows show it.
// Only a visit the user chose reads: a result or manual mark that arrives on
// the current transcript, or a transcript the app selected by itself, stays
// unread until the user leaves and comes back.
export function trackTaskAttention(unread, currentTask, visible, save = () => {}, automatic = () => false) {
  let held = null;
  const stop = watch(
    () => {
      const id = currentTask();
      return [id, id ? unread[id] : null, visible()];
    },
    ([id, turn, shown], previous, cleanup) => {
      if (id !== previous?.[0]) held = turn && automatic() ? id : null;
      else if (turn !== previous?.[1]) held = turn ? id : null;
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
// and a folded project shows no task rows to say so. A scheduled run counts as
// running while its prompt is still queued, so in a paused project it is held
// work, drawn as a clock, rather than a turn in flight.
export function projectDot(project, unread, approvals = {}) {
  const working = project.recent_sessions.some((task) => task.status === "running");
  const scheduled = project.schedules.some((schedule) => schedule.running);
  const running = working || (scheduled && !project.queue_paused);
  const held = project.queue_paused && !working &&
    (scheduled || project.recent_sessions.some((task) => task.queue_count > 0));
  const attention = project.recent_sessions.some((task) =>
    approvals[task.id] || taskDot(task.status, unread[task.id]) === "unread");
  if (attention) return running ? "unread-running" : "unread";
  return running ? "running" : held ? "paused" : null;
}
