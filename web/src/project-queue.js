// The same project action appears in its row and its tasks' context menus.
export function projectQueueAction(project) {
  if (!project) return null;
  const tasks = project.recent_sessions || [];
  const running = tasks.some((task) => task.status === "running") ||
    (project.schedules || []).some((schedule) => schedule.running);
  const queued = tasks.some((task) => task.queue_count > 0);
  if (project.queue_paused) {
    return running || queued ? {
      label: "Resume project queue",
      icon: "i-lucide-play",
      hint: "Resume queued work after active tasks finish",
      paused: false,
    } : null;
  }
  return running ? {
    label: "Pause project queue",
    icon: "i-lucide-pause",
    hint: "Pause queued work after active tasks finish",
    paused: true,
  } : null;
}
