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
