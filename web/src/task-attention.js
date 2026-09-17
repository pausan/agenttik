import { watch } from "vue";

// One timer for the visible transcript, independent of how many rows show it.
export function trackTaskAttention(unread, currentTask, visible, save = () => {}) {
  const stop = watch(
    () => {
      const id = currentTask();
      return [id, id ? unread[id] : null, visible()];
    },
    ([id, turn, shown], _, cleanup) => {
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
