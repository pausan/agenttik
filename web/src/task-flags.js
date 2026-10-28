// A task's mark, in the order a click on its dot cycles through.
// See specs/098-task-flags.md.
export const TASK_FLAGS = [
  { value: "star", label: "Star", icon: "i-lucide-star", color: "text-yellow-400 *:fill-current" },
  { value: "warn", label: "Warning", icon: "i-lucide-triangle-alert", color: "text-amber-500" },
  { value: "danger", label: "Danger", icon: "i-lucide-circle-alert", color: "text-error" },
];

export function taskFlag(flag) {
  return TASK_FLAGS.find((f) => f.value === flag) || null;
}

// No mark, then each mark in turn, then no mark again.
export function nextFlag(flag) {
  const i = TASK_FLAGS.findIndex((f) => f.value === flag);
  return i + 1 < TASK_FLAGS.length ? TASK_FLAGS[i + 1].value : "";
}

// Where a click on the dot goes: a read, unmarked task turns unread first,
// then the marks follow, and clearing the last mark reads it again. unread is
// 0 when read, -1 when marked by hand, or the finished turn's ID; an unmarked
// task left unread by a finished turn is only read, so the cycle starts over.
export function nextDot(flag, unread) {
  if (!taskFlag(flag) && unread > 0) return { flag: "", unread: false };
  if (!taskFlag(flag) && !unread) return { flag: "", unread: true };
  const next = nextFlag(flag);
  return { flag: next, unread: !!next };
}
