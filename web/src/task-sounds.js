import { ref } from "vue";

export const TASK_SOUNDS_KEY = "agenttik.taskSounds";

// Keep audio local to this window and allocate it only after opting in.
export function createTaskSounds(storage, AudioContext = globalThis.AudioContext || globalThis.webkitAudioContext) {
  const enabled = ref(false);
  const seen = new Set();
  let context;
  function unlock() {
    if (!enabled.value || !AudioContext) return;
    try {
      context ||= new AudioContext();
      if (context.state === "suspended") context.resume().catch(() => {});
    } catch { /* Audio may be unavailable in this browser. */ }
  }
  function setEnabled(value) {
    enabled.value = value === true;
    try { storage.setItem(TASK_SOUNDS_KEY, enabled.value ? "1" : "0"); } catch { /* memory still works */ }
    if (enabled.value) unlock();
  }
  function load() {
    try { enabled.value = storage.getItem(TASK_SOUNDS_KEY) === "1"; } catch { /* default stays off */ }
  }
  function onEvent(msg) {
    const type = msg.event?.type;
    if (type !== "error" && !(type === "done" && msg.stats)) return;
    if (!msg.session_id || !msg.turn_id) return;
    // Error and final completion belong to the same alert, even when other
    // tasks' events arrive between duplicate project/session deliveries.
    const key = `${msg.session_id}:${msg.turn_id}`;
    if (seen.has(key)) return;
    seen.add(key);
    if (seen.size > 1024) seen.delete(seen.values().next().value);
    if (!enabled.value) return;
    unlock();
    if (context?.state !== "running") return;
    try {
      const tone = context.createOscillator();
      const volume = context.createGain();
      const now = context.currentTime;
      tone.frequency.value = 880;
      volume.gain.setValueAtTime(0, now);
      volume.gain.linearRampToValueAtTime(0.15, now + 0.01);
      volume.gain.exponentialRampToValueAtTime(0.001, now + 0.65);
      tone.connect(volume);
      volume.connect(context.destination);
      tone.onended = () => { tone.disconnect(); volume.disconnect(); };
      tone.start(now);
      tone.stop(now + 0.7);
    } catch { /* Sound must never interrupt task updates. */ }
  }
  return { enabled, setEnabled, load, unlock, onEvent };
}
